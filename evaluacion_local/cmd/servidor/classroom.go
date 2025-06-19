package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

// ClientInfo información de un cliente/estudiante conectado
type ClientInfo struct {
	ClientID       string         `json:"client_id"`
	ComputerName   string         `json:"computer_name"`
	StudentName    string         `json:"student_name,omitempty"`
	StudentID      string         `json:"student_id,omitempty"`
	IP             string         `json:"ip"`
	MAC            string         `json:"mac,omitempty"`
	LastSeen       time.Time      `json:"last_seen"`
	Status         string         `json:"status"` // "connected", "in_exam", "disconnected", "idle"
	CurrentExam    string         `json:"current_exam,omitempty"`
	ExamStartTime  int64          `json:"exam_start_time,omitempty"`
	ComputerSpecs  *ComputerSpecs `json:"computer_specs,omitempty"`
	NetworkLatency int            `json:"network_latency_ms"`
}

// ComputerSpecs especificaciones del equipo del estudiante
type ComputerSpecs struct {
	OS               string `json:"os"`
	Architecture     string `json:"architecture"`
	Memory           string `json:"memory"`
	Processor        string `json:"processor"`
	BrowserInfo      string `json:"browser_info,omitempty"`
	ScreenResolution string `json:"screen_resolution,omitempty"`
}

// ClientMessage mensaje que envía un cliente al servidor
type ClientMessage struct {
	Type      string      `json:"type"`   // "hello", "heartbeat", "status_update", "goodbye"
	Action    string      `json:"action"` // "join", "ping", "exam_start", "exam_end", "leave"
	ClientID  string      `json:"client_id"`
	Data      *ClientInfo `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

// ServerResponse respuesta del servidor a un cliente
type ServerResponse struct {
	Type         string      `json:"type"`   // "welcome", "pong", "command", "error"
	Action       string      `json:"action"` // "accepted", "rejected", "exam_assigned", "shutdown"
	Message      string      `json:"message,omitempty"`
	ServerInfo   *ServerInfo `json:"server_info,omitempty"`
	AssignedExam string      `json:"assigned_exam,omitempty"`
	Timestamp    int64       `json:"timestamp"`
}

// ClassroomManager maneja la detección y gestión de estudiantes
type ClassroomManager struct {
	server       *DiscoveryServer
	clients      map[string]*ClientInfo
	clientsMutex sync.RWMutex
	udpListener  *net.UDPConn
	running      bool
	logger       *log.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup

	// Configuración
	clientTimeout time.Duration
	maxClients    int
	requireAuth   bool
}

// NewClassroomManager crea un nuevo gestor de sala
func NewClassroomManager(server *DiscoveryServer) *ClassroomManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &ClassroomManager{
		server:        server,
		clients:       make(map[string]*ClientInfo),
		logger:        log.New(os.Stdout, "[CLASSROOM] ", log.LstdFlags|log.Lshortfile),
		ctx:           ctx,
		cancel:        cancel,
		clientTimeout: 30 * time.Second,
		maxClients:    server.config.MaxStudents, // Típico para una sala de informática
		requireAuth:   false,                     // Cambiar según necesidades
	}
}

// Start inicia la detección de estudiantes
func (cm *ClassroomManager) Start() error {
	if cm.running {
		return fmt.Errorf("classroom manager ya está ejecutándose")
	}

	cm.running = true
	cm.logger.Printf("Iniciando detección de estudiantes...")

	// Configurar listener UDP para recibir mensajes de clientes
	if err := cm.startUDPListener(); err != nil {
		return fmt.Errorf("error iniciando listener UDP: %v", err)
	}

	// Iniciar limpieza periódica de clientes inactivos
	cm.wg.Add(1)
	go cm.clientCleanupLoop()

	// Iniciar reporte de estado periódico
	cm.wg.Add(1)
	go cm.statusReportLoop()

	cm.logger.Printf("Detección de estudiantes iniciada. Esperando conexiones...")
	return nil
}

// startUDPListener inicia el listener para mensajes de clientes
func (cm *ClassroomManager) startUDPListener() error {
	// Usar el mismo puerto pero escuchar en todas las interfaces
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", cm.server.config.MulticastPort))
	if err != nil {
		return fmt.Errorf("error resolviendo dirección: %v", err)
	}

	// Crear listener UDP
	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("error creando listener UDP: %v", err)
	}

	cm.udpListener = listener

	// Iniciar goroutine para manejar mensajes entrantes
	cm.wg.Add(1)
	go cm.handleIncomingMessages()

	return nil
}

// handleIncomingMessages maneja los mensajes entrantes de los clientes
func (cm *ClassroomManager) handleIncomingMessages() {
	defer cm.wg.Done()

	buffer := make([]byte, 2048)

	for {
		select {
		case <-cm.ctx.Done():
			return
		default:
			// Establecer timeout para evitar bloqueo
			cm.udpListener.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, clientAddr, err := cm.udpListener.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout normal, continuar
				}
				cm.logger.Printf("Error leyendo mensaje UDP: %v", err)
				continue
			}

			// Procesar mensaje
			go cm.processClientMessage(buffer[:n], clientAddr)
		}
	}
}

// processClientMessage procesa un mensaje de cliente
func (cm *ClassroomManager) processClientMessage(data []byte, clientAddr *net.UDPAddr) {
	var message ClientMessage
	if err := json.Unmarshal(data, &message); err != nil {
		cm.logger.Printf("Error deserializando mensaje de %s: %v", clientAddr.IP, err)
		return
	}

	// Validar mensaje
	if message.ClientID == "" {
		cm.logger.Printf("Mensaje sin ClientID de %s", clientAddr.IP)
		return
	}

	cm.logger.Printf("Mensaje recibido de %s:%d: Type=%s, Action=%s, ClientID=%s",
		clientAddr.IP, clientAddr.Port, message.Type, message.Action, message.ClientID)

	// Procesar según tipo de mensaje
	switch message.Type {
	case "hello":
		cm.handleClientHello(&message, clientAddr)
	case "heartbeat":
		cm.handleClientHeartbeat(&message, clientAddr)
	case "status_update":
		cm.handleClientStatusUpdate(&message, clientAddr)
	case "goodbye":
		cm.handleClientGoodbye(&message, clientAddr)
	default:
		cm.logger.Printf("Tipo de mensaje desconocido: %s de %s", message.Type, clientAddr.IP)
	}
}

// handleClientHello maneja cuando un cliente se conecta por primera vez
func (cm *ClassroomManager) handleClientHello(message *ClientMessage, clientAddr *net.UDPAddr) {
	cm.clientsMutex.Lock()
	defer cm.clientsMutex.Unlock()

	responseAddr := &net.UDPAddr{
		IP:   clientAddr.IP,
		Port: 15000, // Puerto origen del cliente
	}

	// Verificar si ya tenemos este cliente
	if existingClient, exists := cm.clients[message.ClientID]; exists {
		cm.logger.Printf("Cliente %s ya existe, actualizando información", message.ClientID)
		existingClient.LastSeen = time.Now()
		existingClient.IP = clientAddr.IP.String()
		existingClient.Status = "connected"
		cm.sendWelcomeResponse(responseAddr, message.ClientID, "reconnected")
		return
	}

	// Verificar límite de clientes
	if len(cm.clients) >= cm.maxClients {
		cm.logger.Printf("Límite de clientes alcanzado (%d), rechazando %s", cm.maxClients, message.ClientID)
		cm.sendErrorResponse(responseAddr, "classroom_full", "Sala llena, intente más tarde")
		return
	}

	// Crear nuevo cliente
	client := &ClientInfo{
		ClientID:       message.ClientID,
		ComputerName:   message.Data.ComputerName,
		StudentName:    message.Data.StudentName,
		StudentID:      message.Data.StudentID,
		IP:             clientAddr.IP.String(),
		MAC:            message.Data.MAC,
		LastSeen:       time.Now(),
		Status:         "connected",
		ComputerSpecs:  message.Data.ComputerSpecs,
		NetworkLatency: cm.calculateLatency(clientAddr),
	}

	cm.clients[message.ClientID] = client

	cm.logger.Printf("Nuevo cliente conectado: %s (%s) desde %s",
		client.ComputerName, client.StudentName, client.IP)

	// Enviar respuesta de bienvenida
	cm.sendWelcomeResponse(responseAddr, message.ClientID, "welcome")
}

// Funciones de respuesta
func (cm *ClassroomManager) sendWelcomeResponse(clientAddr *net.UDPAddr, clientID, action string) {
	response := &ServerResponse{
		Type:       "response",
		Action:     action,
		Message:    fmt.Sprintf("Bienvenido a %s", cm.server.config.ServerName),
		ServerInfo: cm.server.GetServerInfo(),
		Timestamp:  time.Now().Unix(),
	}
	cm.sendResponse(clientAddr, response)
}

// TODO: Voy acá en la revisión del código

// handleClientHeartbeat maneja los mensajes de heartbeat
func (cm *ClassroomManager) handleClientHeartbeat(message *ClientMessage, clientAddr *net.UDPAddr) {
	cm.clientsMutex.Lock()
	defer cm.clientsMutex.Unlock()

	if client, exists := cm.clients[message.ClientID]; exists {
		client.LastSeen = time.Now()
		client.NetworkLatency = cm.calculateLatency(clientAddr)

		// Responder con pong
		cm.sendPongResponse(clientAddr, message.ClientID)
	} else {
		// Cliente no registrado, pedirle que se registre
		cm.sendErrorResponse(clientAddr, "not_registered", "Cliente no registrado, envíe mensaje 'hello'")
	}
}

// handleClientStatusUpdate maneja actualizaciones de estado
func (cm *ClassroomManager) handleClientStatusUpdate(message *ClientMessage, clientAddr *net.UDPAddr) {
	cm.clientsMutex.Lock()
	defer cm.clientsMutex.Unlock()

	if client, exists := cm.clients[message.ClientID]; exists {
		client.LastSeen = time.Now()

		// Actualizar información específica
		if message.Data != nil {
			if message.Data.Status != "" {
				client.Status = message.Data.Status
			}
			if message.Data.CurrentExam != "" {
				client.CurrentExam = message.Data.CurrentExam
			}
			if message.Data.ExamStartTime > 0 {
				client.ExamStartTime = message.Data.ExamStartTime
			}
			if message.Data.StudentName != "" {
				client.StudentName = message.Data.StudentName
			}
			if message.Data.StudentID != "" {
				client.StudentID = message.Data.StudentID
			}
		}

		cm.logger.Printf("Estado actualizado para %s: Status=%s, Exam=%s",
			client.ClientID, client.Status, client.CurrentExam)

		// Confirmar actualización
		cm.sendAckResponse(clientAddr, message.ClientID, "status_updated")
	}
}

// handleClientGoodbye maneja cuando un cliente se desconecta
func (cm *ClassroomManager) handleClientGoodbye(message *ClientMessage, clientAddr *net.UDPAddr) {
	cm.clientsMutex.Lock()
	defer cm.clientsMutex.Unlock()

	if client, exists := cm.clients[message.ClientID]; exists {
		cm.logger.Printf("Cliente desconectándose: %s (%s)", client.ComputerName, client.StudentName)
		delete(cm.clients, message.ClientID)

		// Confirmar desconexión
		cm.sendAckResponse(clientAddr, message.ClientID, "goodbye_confirmed")
	}
}

func (cm *ClassroomManager) sendPongResponse(clientAddr *net.UDPAddr, clientID string) {
	response := &ServerResponse{
		Type:      "pong",
		Action:    "heartbeat_ack",
		Timestamp: time.Now().Unix(),
	}
	cm.sendResponse(clientAddr, response)
}

func (cm *ClassroomManager) sendErrorResponse(clientAddr *net.UDPAddr, action, message string) {
	response := &ServerResponse{
		Type:      "error",
		Action:    action,
		Message:   message,
		Timestamp: time.Now().Unix(),
	}
	cm.sendResponse(clientAddr, response)
}

func (cm *ClassroomManager) sendAckResponse(clientAddr *net.UDPAddr, clientID, action string) {
	response := &ServerResponse{
		Type:      "ack",
		Action:    action,
		Timestamp: time.Now().Unix(),
	}
	cm.sendResponse(clientAddr, response)
}

func (cm *ClassroomManager) sendResponse(clientAddr *net.UDPAddr, response *ServerResponse) {
	data, err := json.Marshal(response)
	if err != nil {
		cm.logger.Printf("Error serializando respuesta: %v", err)
		return
	}

	// Crear conexión temporal para responder
	conn, err := net.DialUDP("udp", nil, clientAddr)
	if err != nil {
		cm.logger.Printf("Error creando conexión de respuesta: %v", err)
		return
	}
	defer conn.Close()

	_, err = conn.Write(data)
	if err != nil {
		cm.logger.Printf("Error enviando respuesta: %v", err)
	}

	/*
		// Enviar usando el listener UDP ya existente
		_, err = cm.udpListener.WriteToUDP(data, clientAddr)
		if err != nil {
			cm.logger.Printf("Error enviando respuesta UDP: %v", err)
		} else {
			cm.logger.Printf("Respuesta enviada a %s:%d -> %s/%s", clientAddr.IP, clientAddr.Port, response.Type, response.Action)
		}
	*/

	cm.logger.Printf("Enviando respuesta a %s:%d -> Tipo=%s, Acción=%s, Mensaje=%s",
		clientAddr.IP.String(), clientAddr.Port, response.Type, response.Action, response.Message)
}

// calculateLatency calcula la latencia de red
func (cm *ClassroomManager) calculateLatency(clientAddr *net.UDPAddr) int {
	start := time.Now()
	// Implementar ping real o medición de respuesta
	conn, err := net.DialTimeout("udp", clientAddr.String(), 1*time.Second)
	if err != nil {
		return -1
	}
	defer conn.Close()
	return int(time.Since(start).Milliseconds())
}

// clientCleanupLoop limpia clientes inactivos periódicamente
func (cm *ClassroomManager) clientCleanupLoop() {
	defer cm.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			cm.cleanupInactiveClients()
		}
	}
}

// cleanupInactiveClients elimina clientes que no han enviado heartbeat
func (cm *ClassroomManager) cleanupInactiveClients() {
	cm.clientsMutex.Lock()
	defer cm.clientsMutex.Unlock()

	now := time.Now()
	var disconnected []string

	for clientID, client := range cm.clients {
		if now.Sub(client.LastSeen) > cm.clientTimeout {
			cm.logger.Printf("Cliente inactivo detectado: %s (%s)", client.ComputerName, client.StudentName)
			disconnected = append(disconnected, clientID)
		}
	}

	// Eliminar clientes desconectados
	for _, clientID := range disconnected {
		delete(cm.clients, clientID)
	}

	if len(disconnected) > 0 {
		cm.logger.Printf("Eliminados %d clientes inactivos", len(disconnected))
	}
}

// statusReportLoop reporta el estado de la sala periódicamente
func (cm *ClassroomManager) statusReportLoop() {
	defer cm.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			cm.logClassroomStatus()
		}
	}
}

// logClassroomStatus registra el estado actual de la sala
func (cm *ClassroomManager) logClassroomStatus() {
	cm.clientsMutex.RLock()
	defer cm.clientsMutex.RUnlock()

	total := len(cm.clients)
	inExam := 0
	idle := 0

	for _, client := range cm.clients {
		switch client.Status {
		case "in_exam":
			inExam++
		case "idle", "connected":
			idle++
		}
	}

	cm.logger.Printf("Estado de la sala: Total=%d, En examen=%d, Disponibles=%d",
		total, inExam, idle)
}

// Métodos públicos para obtener información

// GetConnectedClients retorna todos los clientes conectados
func (cm *ClassroomManager) GetConnectedClients() map[string]*ClientInfo {
	cm.clientsMutex.RLock()
	defer cm.clientsMutex.RUnlock()

	// Crear copia para evitar modificaciones concurrentes
	clients := make(map[string]*ClientInfo)
	for k, v := range cm.clients {
		clientCopy := *v
		clients[k] = &clientCopy
	}

	return clients
}

// GetClientCount retorna el número de clientes conectados
func (cm *ClassroomManager) GetClientCount() int {
	cm.clientsMutex.RLock()
	defer cm.clientsMutex.RUnlock()
	return len(cm.clients)
}

// GetClientsByStatus retorna clientes filtrados por estado
func (cm *ClassroomManager) GetClientsByStatus(status string) []*ClientInfo {
	cm.clientsMutex.RLock()
	defer cm.clientsMutex.RUnlock()

	var filtered []*ClientInfo
	for _, client := range cm.clients {
		if client.Status == status {
			clientCopy := *client
			filtered = append(filtered, &clientCopy)
		}
	}

	return filtered
}

// AssignExamToClient asigna un examen a un cliente específico
func (cm *ClassroomManager) AssignExamToClient(clientID, examName string) error {
	cm.clientsMutex.Lock()
	defer cm.clientsMutex.Unlock()

	client, exists := cm.clients[clientID]
	if !exists {
		return fmt.Errorf("cliente %s no encontrado", clientID)
	}

	client.CurrentExam = examName
	client.Status = "in_exam"
	client.ExamStartTime = time.Now().Unix()

	cm.logger.Printf("Examen '%s' asignado a cliente %s (%s)",
		examName, client.ComputerName, client.StudentName)

	return nil
}

// Stop detiene el classroom manager
func (cm *ClassroomManager) Stop() error {
	if !cm.running {
		return nil
	}

	cm.logger.Printf("Deteniendo classroom manager...")
	cm.running = false

	// Cancelar contexto
	cm.cancel()

	// Cerrar listener UDP
	if cm.udpListener != nil {
		cm.udpListener.Close()
	}

	// Esperar goroutines
	cm.wg.Wait()

	cm.logger.Printf("Classroom manager detenido")
	return nil
}
