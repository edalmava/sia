package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Estructuras de datos (copiadas del servidor para compatibilidad)

type ServerInfo struct {
	ServerName   string   `json:"server_name"`
	Institution  string   `json:"institution"`
	Classroom    string   `json:"classroom"`
	Version      string   `json:"version"`
	IP           string   `json:"ip"`
	HTTPPort     int      `json:"http_port"`
	HTTPSPort    int      `json:"https_port"`
	UDPPort      int      `json:"udp_port"`
	Timestamp    int64    `json:"timestamp"`
	ServerID     string   `json:"server_id"`
	Capabilities []string `json:"capabilities"`
	MaxStudents  int      `json:"max_students"`
	ActiveExams  []string `json:"active_exams"`
}

type UDPMessage struct {
	Type      string      `json:"type"`
	Action    string      `json:"action"`
	Data      *ServerInfo `json:"data"`
	Signature string      `json:"signature,omitempty"`
}

type ComputerSpecs struct {
	OS               string `json:"os"`
	Architecture     string `json:"architecture"`
	Memory           string `json:"memory"`
	Processor        string `json:"processor"`
	BrowserInfo      string `json:"browser_info,omitempty"`
	ScreenResolution string `json:"screen_resolution,omitempty"`
}

type ClientInfo struct {
	ClientID       string         `json:"client_id"`
	ComputerName   string         `json:"computer_name"`
	StudentName    string         `json:"student_name,omitempty"`
	StudentID      string         `json:"student_id,omitempty"`
	IP             string         `json:"ip"`
	MAC            string         `json:"mac,omitempty"`
	LastSeen       time.Time      `json:"last_seen"`
	Status         string         `json:"status"`
	CurrentExam    string         `json:"current_exam,omitempty"`
	ExamStartTime  int64          `json:"exam_start_time,omitempty"`
	ComputerSpecs  *ComputerSpecs `json:"computer_specs,omitempty"`
	NetworkLatency int            `json:"network_latency_ms"`
}

type ClientMessage struct {
	Type      string      `json:"type"`
	Action    string      `json:"action"`
	ClientID  string      `json:"client_id"`
	Data      *ClientInfo `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

type ServerResponse struct {
	Type         string      `json:"type"`
	Action       string      `json:"action"`
	Message      string      `json:"message,omitempty"`
	ServerInfo   *ServerInfo `json:"server_info,omitempty"`
	AssignedExam string      `json:"assigned_exam,omitempty"`
	Timestamp    int64       `json:"timestamp"`
}

// Cliente estudiante
type StudentClient struct {
	clientID          string
	computerName      string
	studentName       string
	studentID         string
	status            string
	discoveredServers map[string]*ServerInfo
	selectedServer    *ServerInfo
	multicastConn     *net.UDPConn
	serverConn        *net.UDPConn
	running           bool
	logger            *log.Logger
	ctx               context.Context
	cancel            context.CancelFunc
}

// NewStudentClient crea un nuevo cliente estudiante
func NewStudentClient() (*StudentClient, error) {
	hostname, _ := os.Hostname()
	clientID := fmt.Sprintf("%s-%d", hostname, time.Now().Unix())

	ctx, cancel := context.WithCancel(context.Background())

	return &StudentClient{
		clientID:          clientID,
		computerName:      hostname,
		status:            "disconnected",
		discoveredServers: make(map[string]*ServerInfo),
		logger:            log.New(os.Stdout, "[CLIENTE] ", log.LstdFlags|log.Lshortfile),
		ctx:               ctx,
		cancel:            cancel,
	}, nil
}

// SetStudentInfo establece la información del estudiante
func (sc *StudentClient) SetStudentInfo(name, id string) {
	sc.studentName = name
	sc.studentID = id
	sc.logger.Printf("Información del estudiante establecida: %s (%s)", name, id)
}

// Start inicia el cliente
func (sc *StudentClient) Start() error {
	if sc.running {
		return fmt.Errorf("cliente ya está ejecutándose")
	}

	sc.running = true
	sc.logger.Printf("Iniciando cliente estudiante: %s", sc.clientID)

	// Fase 1: Descubrimiento de servidores
	if err := sc.startDiscovery(); err != nil {
		return fmt.Errorf("error iniciando descubrimiento: %v", err)
	}

	// Esperar un poco para descubrir servidores
	sc.logger.Printf("Buscando servidores disponibles...")
	time.Sleep(8 * time.Second)

	// Fase 2: Seleccionar servidor y conectar
	if err := sc.selectAndConnect(); err != nil {
		return fmt.Errorf("error conectando al servidor: %v", err)
	}

	// Fase 3: Iniciar heartbeat DESPUÉS de conectar
	go sc.heartbeatLoop()

	// Fase 4: Escuchar respuestas del servidor
	go sc.listenServerResponses()

	return nil
}

// startDiscovery inicia la escucha de broadcasts multicast
func (sc *StudentClient) startDiscovery() error {
	// Configurar dirección multicast
	multicastAddr, err := net.ResolveUDPAddr("udp", "224.0.0.100:15000")
	if err != nil {
		return fmt.Errorf("error resolviendo dirección multicast: %v", err)
	}

	// Crear conexión multicast
	conn, err := net.ListenMulticastUDP("udp", nil, multicastAddr)
	if err != nil {
		return fmt.Errorf("error creando listener multicast: %v", err)
	}

	sc.multicastConn = conn

	// Iniciar goroutine para escuchar broadcasts
	go sc.listenBroadcasts()

	sc.logger.Printf("Escuchando broadcasts en 224.0.0.100:15000")
	return nil
}

// listenBroadcasts escucha los broadcasts del servidor
func (sc *StudentClient) listenBroadcasts() {
	buffer := make([]byte, 2048)

	for sc.running {
		sc.multicastConn.SetReadDeadline(time.Now().Add(2 * time.Second))

		n, addr, err := sc.multicastConn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			sc.logger.Printf("Error leyendo broadcast: %v", err)
			continue
		}

		sc.processBroadcast(buffer[:n], addr)
	}
}

// processBroadcast procesa un mensaje broadcast del servidor
func (sc *StudentClient) processBroadcast(data []byte, addr *net.UDPAddr) {
	var message UDPMessage
	if err := json.Unmarshal(data, &message); err != nil {
		sc.logger.Printf("Error parseando broadcast: %v", err)
		return
	}

	if message.Type == "broadcast" && message.Action == "hello" && message.Data != nil {
		serverInfo := message.Data
		serverKey := fmt.Sprintf("%s-%s", serverInfo.IP, serverInfo.ServerID)

		// Solo mostrar cuando es un servidor nuevo
		if _, exists := sc.discoveredServers[serverKey]; !exists {
			sc.logger.Printf("Servidor descubierto: %s (%s) - IP: %s",
				serverInfo.ServerName, serverInfo.Institution, serverInfo.IP)
		}

		sc.discoveredServers[serverKey] = serverInfo
	}
}

// selectAndConnect selecciona un servidor y se conecta
func (sc *StudentClient) selectAndConnect() error {
	if len(sc.discoveredServers) == 0 {
		return fmt.Errorf("no se encontraron servidores disponibles")
	}

	// Seleccionar el primer servidor disponible (en una implementación real,
	// podrías mostrar una lista al usuario)
	for _, server := range sc.discoveredServers {
		sc.selectedServer = server
		break
	}

	sc.logger.Printf("Conectando a: %s (%s)",
		sc.selectedServer.ServerName, sc.selectedServer.IP)

	// Crear conexión UDP al servidor
	serverAddr, err := net.ResolveUDPAddr("udp",
		fmt.Sprintf("%s:%d", sc.selectedServer.IP, sc.selectedServer.UDPPort))
	if err != nil {
		return fmt.Errorf("error resolviendo dirección del servidor: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return fmt.Errorf("error creando conexión al servidor: %v", err)
	}

	sc.serverConn = conn

	// Enviar mensaje de registro (hello) y establecer estado
	if err := sc.sendHelloMessage(); err != nil {
		return err
	}

	// Cambiar estado a connecting mientras esperamos respuesta
	sc.status = "connecting"
	sc.logger.Printf("Estado cambiado a: connecting")

	return nil
}

// sendHelloMessage envía el mensaje de registro inicial
func (sc *StudentClient) sendHelloMessage() error {
	clientInfo := &ClientInfo{
		ClientID:      sc.clientID,
		ComputerName:  sc.computerName,
		StudentName:   sc.studentName,
		StudentID:     sc.studentID,
		Status:        "connecting", // Estado inicial
		ComputerSpecs: sc.getComputerSpecs(),
	}

	message := &ClientMessage{
		Type:      "hello",
		Action:    "join",
		ClientID:  sc.clientID,
		Data:      clientInfo,
		Timestamp: time.Now().Unix(),
	}

	return sc.sendMessage(message)
}

// getComputerSpecs obtiene las especificaciones del equipo
func (sc *StudentClient) getComputerSpecs() *ComputerSpecs {
	return &ComputerSpecs{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Memory:       "N/A", // En una implementación real, obtener memoria real
		Processor:    "N/A", // En una implementación real, obtener CPU real
		BrowserInfo:  "Go Client 1.0",
	}
}

// sendMessage envía un mensaje al servidor
func (sc *StudentClient) sendMessage(message *ClientMessage) error {
	if sc.serverConn == nil {
		return fmt.Errorf("no hay conexión al servidor")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error serializando mensaje: %v", err)
	}

	_, err = sc.serverConn.Write(data)
	if err != nil {
		return fmt.Errorf("error enviando mensaje: %v", err)
	}

	sc.logger.Printf("Mensaje enviado: %s/%s", message.Type, message.Action)
	return nil
}

// heartbeatLoop mantiene la conexión con heartbeats periódicos
func (sc *StudentClient) heartbeatLoop() {
	sc.logger.Printf("🔄 Iniciando heartbeat loop...")
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sc.ctx.Done():
			sc.logger.Printf("🔄 Heartbeat loop terminado por contexto")
			return
		case <-ticker.C:
			sc.logger.Printf("🔄 Heartbeat tick - Estado actual: %s", sc.status)

			// CORECCIÓN: Incluir más estados válidos para heartbeat
			if sc.status == "connected" || sc.status == "in_exam" || sc.status == "idle" || sc.status == "connecting" {
				if err := sc.sendHeartbeat(); err != nil {
					sc.logger.Printf("❌ Error enviando heartbeat: %v", err)
				} else {
					sc.logger.Printf("💓 Heartbeat enviado exitosamente")
				}
			} else {
				sc.logger.Printf("⏸️  Heartbeat omitido - Estado: %s", sc.status)
			}
		}
	}
}

// sendHeartbeat envía un mensaje de heartbeat
func (sc *StudentClient) sendHeartbeat() error {
	// Crear información básica del cliente para el heartbeat
	clientInfo := &ClientInfo{
		ClientID:     sc.clientID,
		ComputerName: sc.computerName,
		StudentName:  sc.studentName,
		StudentID:    sc.studentID,
		Status:       sc.status,
		LastSeen:     time.Now(),
	}

	message := &ClientMessage{
		Type:      "heartbeat",
		Action:    "ping",
		ClientID:  sc.clientID,
		Data:      clientInfo, // CORECCIÓN: Incluir datos del cliente
		Timestamp: time.Now().Unix(),
	}

	return sc.sendMessage(message)
}

// listenServerResponses escucha las respuestas del servidor
func (sc *StudentClient) listenServerResponses() {
	if sc.serverConn == nil {
		return
	}

	buffer := make([]byte, 2048)

	for sc.running {
		sc.serverConn.SetReadDeadline(time.Now().Add(2 * time.Second))

		n, err := sc.serverConn.Read(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			sc.logger.Printf("Error leyendo respuesta del servidor: %v", err)
			continue
		}

		sc.processServerResponse(buffer[:n])
	}
}

// processServerResponse procesa una respuesta del servidor
func (sc *StudentClient) processServerResponse(data []byte) {
	var response ServerResponse
	if err := json.Unmarshal(data, &response); err != nil {
		sc.logger.Printf("Error parseando respuesta del servidor: %v", err)
		return
	}

	switch response.Type {
	case "response":
		sc.handleServerResponse(&response)
	case "error":
		sc.handleServerError(&response)
	case "ack":
		sc.logger.Printf("Confirmación recibida: %s", response.Action)
	default:
		sc.logger.Printf("Tipo de respuesta desconocido: %s", response.Type)
	}
}

// handleServerResponse maneja respuestas exitosas del servidor
func (sc *StudentClient) handleServerResponse(response *ServerResponse) {
	switch response.Action {
	case "welcome", "reconnected":
		sc.status = "connected" // CORECCIÓN: Asegurar cambio de estado
		sc.logger.Printf("✅ Conectado exitosamente: %s", response.Message)
		sc.logger.Printf("🔄 Estado actualizado a: %s", sc.status)
		if response.ServerInfo != nil {
			sc.logger.Printf("Servidor: %s - Estudiantes máximos: %d",
				response.ServerInfo.ServerName, response.ServerInfo.MaxStudents)
		}
	case "pong":
		sc.logger.Printf("🏓 Pong recibido del servidor")
		// Heartbeat confirmado, mantener estado actual
	default:
		sc.logger.Printf("Respuesta del servidor: %s - %s", response.Action, response.Message)
	}
}

// handleServerError maneja errores del servidor
func (sc *StudentClient) handleServerError(response *ServerResponse) {
	switch response.Action {
	case "classroom_full":
		sc.logger.Printf("❌ Sala llena: %s", response.Message)
		sc.status = "disconnected"
	case "not_registered":
		sc.logger.Printf("⚠️  No registrado, reintentando registro...")
		time.Sleep(2 * time.Second)
		sc.sendHelloMessage()
	default:
		sc.logger.Printf("❌ Error del servidor: %s - %s", response.Action, response.Message)
	}
}

// UpdateStatus actualiza el estado del cliente
func (sc *StudentClient) UpdateStatus(newStatus, examName string) error {
	oldStatus := sc.status
	sc.status = newStatus

	clientInfo := &ClientInfo{
		ClientID:    sc.clientID,
		Status:      newStatus,
		CurrentExam: examName,
	}

	if newStatus == "in_exam" {
		clientInfo.ExamStartTime = time.Now().Unix()
	}

	message := &ClientMessage{
		Type:      "status_update",
		Action:    "status_change",
		ClientID:  sc.clientID,
		Data:      clientInfo,
		Timestamp: time.Now().Unix(),
	}

	sc.logger.Printf("🔄 Actualizando estado: %s -> %s", oldStatus, newStatus)
	return sc.sendMessage(message)
}

// Stop detiene el cliente
func (sc *StudentClient) Stop() error {
	if !sc.running {
		return nil
	}

	sc.logger.Printf("Deteniendo cliente...")
	sc.running = false

	// Enviar mensaje de despedida
	if sc.serverConn != nil {
		message := &ClientMessage{
			Type:      "goodbye",
			Action:    "leave",
			ClientID:  sc.clientID,
			Data:      nil,
			Timestamp: time.Now().Unix(),
		}
		sc.sendMessage(message)
	}

	// Cancelar contexto
	sc.cancel()

	// Cerrar conexiones
	if sc.multicastConn != nil {
		sc.multicastConn.Close()
	}
	if sc.serverConn != nil {
		sc.serverConn.Close()
	}

	sc.logger.Printf("Cliente detenido")
	return nil
}

// ShowStatus muestra el estado actual del cliente
func (sc *StudentClient) ShowStatus() {
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Printf("ESTADO DEL CLIENTE\n")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("ID del Cliente: %s\n", sc.clientID)
	fmt.Printf("Nombre del Equipo: %s\n", sc.computerName)
	fmt.Printf("Estudiante: %s (%s)\n", sc.studentName, sc.studentID)
	fmt.Printf("Estado: %s\n", sc.status)

	if sc.selectedServer != nil {
		fmt.Printf("Servidor Conectado: %s\n", sc.selectedServer.ServerName)
		fmt.Printf("IP del Servidor: %s\n", sc.selectedServer.IP)
	}

	fmt.Printf("Servidores Descubiertos: %d\n", len(sc.discoveredServers))
	fmt.Println(strings.Repeat("=", 50))
}

func main() {
	fmt.Println("=== CLIENTE ESTUDIANTE - SISTEMA DE EVALUACIONES ===")

	// Crear cliente
	client, err := NewStudentClient()
	if err != nil {
		log.Fatalf("Error creando cliente: %v", err)
	}

	// Pedir información del estudiante (opcional)
	fmt.Print("Ingrese su nombre (Enter para omitir): ")
	var studentName string
	fmt.Scanln(&studentName)

	fmt.Print("Ingrese su ID de estudiante (Enter para omitir): ")
	var studentID string
	fmt.Scanln(&studentID)

	if studentName != "" || studentID != "" {
		client.SetStudentInfo(studentName, studentID)
	}

	// Configurar manejo de señales
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Iniciar cliente
	if err := client.Start(); err != nil {
		log.Fatalf("Error iniciando cliente: %v", err)
	}

	log.Printf("Cliente iniciado. Presiona Ctrl+C para salir.")

	// Mostrar estado cada 30 segundos
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-client.ctx.Done():
				return
			case <-ticker.C:
				client.ShowStatus()
			}
		}
	}()

	// Simular algunos cambios de estado para demostración
	go func() {
		time.Sleep(25 * time.Second) // Esperar más tiempo para asegurar conexión
		if client.status == "connected" {
			client.UpdateStatus("idle", "")
			log.Printf("Estado cambiado a: idle")
		}

		time.Sleep(30 * time.Second)
		if client.status == "idle" {
			client.UpdateStatus("in_exam", "Examen de Prueba")
			log.Printf("Estado cambiado a: in_exam")
		}

		time.Sleep(45 * time.Second)
		if client.status == "in_exam" {
			client.UpdateStatus("connected", "")
			log.Printf("Estado cambiado a: connected")
		}
	}()

	// Esperar señal de terminación
	<-sigChan

	// Shutdown graceful
	log.Printf("Recibida señal de terminación...")
	if err := client.Stop(); err != nil {
		log.Printf("Error durante shutdown: %v", err)
	}

	log.Printf("Cliente terminado exitosamente")
}
