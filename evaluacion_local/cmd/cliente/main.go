package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

// DiscoveryClient maneja el descubrimiento de servidores
type DiscoveryClient struct {
	config        *ClientConfig
	foundServers  map[string]*ServerInfo
	serversMutex  sync.RWMutex
	logger        *log.Logger
	ctx           context.Context
	cancel        context.CancelFunc
	onServerFound func(*ServerInfo)
	onServerLost  func(string)
}

// StudentClient cliente que se ejecuta en el computador del estudiante
type StudentClient struct {
	config     *ClientConfig
	clientInfo *ClientInfo
	serverInfo *ServerInfo

	udpConn *net.UDPConn
	running bool
	logger  *log.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Estado de conexión
	connected         bool
	lastHeartbeat     time.Time
	serverAddr        *net.UDPAddr
	registrationTries int
	maxRetries        int
}

// ClientInfo información del cliente (reutilizada del servidor)
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

// ComputerSpecs especificaciones del equipo
type ComputerSpecs struct {
	OS               string `json:"os"`
	Architecture     string `json:"architecture"`
	Memory           string `json:"memory"`
	Processor        string `json:"processor"`
	BrowserInfo      string `json:"browser_info,omitempty"`
	ScreenResolution string `json:"screen_resolution,omitempty"`
}

// Estructuras de mensajes (reutilizadas del servidor)
type ClientMessage struct {
	Type      string      `json:"type"`
	Action    string      `json:"action"`
	ClientID  string      `json:"client_id"`
	Data      *ClientInfo `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

// ClientConfig configuración del cliente
type ClientConfig struct {
	StudentName            string        `json:"student_name"`
	StudentID              string        `json:"student_id"`
	ServerDiscoveryTimeout time.Duration `json:"server_discovery_timeout"`
	HeartbeatInterval      time.Duration `json:"heartbeat_interval"`
	MulticastAddr          string        `json:"multicast_addr"`
	MulticastPort          int           `json:"multicast_port"`
	MDNSServiceType        string        `json:"mdns_service_type"`
	DiscoveryTimeout       time.Duration `json:"discovery_timeout"`
	EnableMDNS             bool          `json:"enable_mdns"`
	EnableUDPMulticast     bool          `json:"enable_udp_multicast"`
	ServerValidation       bool          `json:"server_validation"`
	AutoReconnect          bool          `json:"auto_reconnect"`
	MaxReconnectTries      int           `json:"max_reconnect_tries"`
}

// ServerInfo información del servidor descubierto
type ServerInfo struct {
	ServerName   string    `json:"server_name"`
	Version      string    `json:"version"`
	IP           string    `json:"ip"`
	HTTPPort     int       `json:"http_port"`
	HTTPSPort    int       `json:"https_port"`
	ServerID     string    `json:"server_id"`
	Capabilities []string  `json:"capabilities"`
	Timestamp    int64     `json:"timestamp"`
	LastSeen     time.Time `json:"last_seen"`
	Source       string    `json:"source"` // "mdns" o "udp"
}

type ServerResponse struct {
	Type         string      `json:"type"`
	Action       string      `json:"action"`
	Message      string      `json:"message,omitempty"`
	ServerInfo   *ServerInfo `json:"server_info,omitempty"`
	AssignedExam string      `json:"assigned_exam,omitempty"`
	Timestamp    int64       `json:"timestamp"`
}

// UDPMessage estructura para mensajes UDP
type UDPMessage struct {
	Type   string      `json:"type"`
	Action string      `json:"action"`
	Data   *ServerInfo `json:"data"`
}

// NewDiscoveryClient crea un nuevo cliente de descubrimiento
func NewDiscoveryClient(config *ClientConfig) *DiscoveryClient {
	ctx, cancel := context.WithCancel(context.Background())

	logger := log.New(os.Stdout, "[DISCOVERY-CLIENT] ", log.LstdFlags|log.Lshortfile)

	return &DiscoveryClient{
		config:       config,
		foundServers: make(map[string]*ServerInfo),
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// SetCallbacks configura callbacks para eventos de descubrimiento
func (dc *DiscoveryClient) SetCallbacks(onFound func(*ServerInfo), onLost func(string)) {
	dc.onServerFound = onFound
	dc.onServerLost = onLost
}

// StartDiscovery inicia el proceso de descubrimiento
func (dc *DiscoveryClient) StartDiscovery() error {
	dc.logger.Printf("Iniciando descubrimiento de servidores...")

	var wg sync.WaitGroup

	// Iniciar descubrimiento mDNS si está habilitado
	if dc.config.EnableMDNS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := dc.startMDNSDiscovery(); err != nil {
				dc.logger.Printf("Error en descubrimiento mDNS: %v", err)
			}
		}()
	}

	// Iniciar descubrimiento UDP multicast si está habilitado
	if dc.config.EnableUDPMulticast {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := dc.startUDPDiscovery(); err != nil {
				dc.logger.Printf("Error en descubrimiento UDP: %v", err)
			}
		}()
	}

	// Iniciar limpieza periódica de servidores obsoletos
	wg.Add(1)
	go func() {
		defer wg.Done()
		dc.startServerCleanup()
	}()

	// Esperar que terminen todas las goroutines
	go func() {
		wg.Wait()
		dc.logger.Printf("Todos los servicios de descubrimiento han terminado")
	}()

	return nil
}

// startMDNSDiscovery inicia el descubrimiento vía mDNS - VERSIÓN CORREGIDA
func (dc *DiscoveryClient) startMDNSDiscovery() error {
	dc.logger.Printf("Iniciando descubrimiento mDNS...")

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("error creando resolver mDNS: %v", err)
	}

	// Usar ticker para búsquedas periódicas
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-dc.ctx.Done():
			dc.logger.Printf("Deteniendo descubrimiento mDNS...")
			return nil
		case <-ticker.C:
			// Realizar una búsqueda individual
			if err := dc.performSingleMDNSSearch(resolver); err != nil {
				dc.logger.Printf("Error en búsqueda mDNS: %v", err)
			}
		}
	}
}

// performSingleMDNSSearch realiza una búsqueda mDNS individual
func (dc *DiscoveryClient) performSingleMDNSSearch(resolver *zeroconf.Resolver) error {
	// Crear un canal nuevo para cada búsqueda
	entries := make(chan *zeroconf.ServiceEntry)

	// Crear contexto con timeout para esta búsqueda específica
	ctx, cancel := context.WithTimeout(dc.ctx, dc.config.DiscoveryTimeout)
	defer cancel()

	// Procesar entradas en una goroutine separada
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case entry, ok := <-entries:
				if !ok {
					// Canal cerrado por zeroconf, terminar
					return
				}
				dc.processMDNSEntry(entry)
			case <-ctx.Done():
				// Timeout o cancelación, terminar
				return
			}
		}
	}()

	// Realizar la búsqueda (zeroconf cerrará el canal automáticamente)
	err := resolver.Browse(ctx, dc.config.MDNSServiceType, "local.", entries)

	// Esperar a que termine el procesamiento o timeout
	select {
	case <-done:
		// Procesamiento completado normalmente
	case <-time.After(dc.config.DiscoveryTimeout + 5*time.Second):
		// Timeout adicional por seguridad
		dc.logger.Printf("Timeout esperando fin de procesamiento mDNS")
	}

	return err
}

// processMDNSEntry procesa una entrada mDNS encontrada
func (dc *DiscoveryClient) processMDNSEntry(entry *zeroconf.ServiceEntry) {
	if len(entry.AddrIPv4) == 0 {
		return
	}

	// Extraer información de los registros TXT
	txtMap := make(map[string]string)
	for _, txt := range entry.Text {
		if len(txt) > 0 {
			parts := []byte(txt)
			for i, b := range parts {
				if b == '=' {
					key := string(parts[:i])
					value := string(parts[i+1:])
					txtMap[key] = value
					break
				}
			}
		}
	}

	serverInfo := &ServerInfo{
		ServerName: entry.Service,
		IP:         entry.AddrIPv4[0].String(),
		HTTPSPort:  entry.Port,
		LastSeen:   time.Now(),
		Source:     "mdns",
		Timestamp:  time.Now().Unix(),
	}

	// Extraer información adicional de TXT records
	if version, ok := txtMap["version"]; ok {
		serverInfo.Version = version
	}
	if serverID, ok := txtMap["server_id"]; ok {
		serverInfo.ServerID = serverID
	}
	if httpPort, ok := txtMap["http_port"]; ok {
		fmt.Sscanf(httpPort, "%d", &serverInfo.HTTPPort)
	}
	if capabilities, ok := txtMap["capabilities"]; ok {
		// Parsear capabilities separadas por comas
		serverInfo.Capabilities = []string{capabilities} // Simplificado
	}

	dc.addOrUpdateServer(serverInfo)
}

// startUDPDiscovery inicia el descubrimiento vía UDP multicast
func (dc *DiscoveryClient) startUDPDiscovery() error {
	dc.logger.Printf("Iniciando descubrimiento UDP multicast...")

	addr, err := net.ResolveUDPAddr("udp",
		fmt.Sprintf("%s:%d", dc.config.MulticastAddr, dc.config.MulticastPort))
	if err != nil {
		return fmt.Errorf("error resolviendo dirección multicast: %v", err)
	}

	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("error creando listener multicast: %v", err)
	}
	defer conn.Close()

	buffer := make([]byte, 2048)

	for {
		select {
		case <-dc.ctx.Done():
			dc.logger.Printf("Deteniendo descubrimiento UDP...")
			return nil
		default:
			// Configurar timeout de lectura
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, src, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout normal, continuar
				}
				dc.logger.Printf("Error leyendo UDP: %v", err)
				continue
			}

			dc.processUDPMessage(buffer[:n], src)
		}
	}
}

// processUDPMessage procesa un mensaje UDP recibido
func (dc *DiscoveryClient) processUDPMessage(data []byte, src *net.UDPAddr) {
	var message UDPMessage
	if err := json.Unmarshal(data, &message); err != nil {
		dc.logger.Printf("Error parseando mensaje UDP desde %s: %v", src, err)
		return
	}

	// Validar tipo de mensaje
	if message.Type != "broadcast" || message.Action != "hello" || message.Data == nil {
		return
	}

	serverInfo := message.Data
	serverInfo.LastSeen = time.Now()
	serverInfo.Source = "udp"

	// Validar que la IP coincida con la fuente
	if serverInfo.IP != src.IP.String() {
		dc.logger.Printf("Advertencia: IP en mensaje (%s) no coincide con fuente (%s)",
			serverInfo.IP, src.IP.String())
		serverInfo.IP = src.IP.String() // Usar IP real
	}

	dc.addOrUpdateServer(serverInfo)
}

// addOrUpdateServer agrega o actualiza información de servidor
func (dc *DiscoveryClient) addOrUpdateServer(serverInfo *ServerInfo) {
	dc.serversMutex.Lock()
	defer dc.serversMutex.Unlock()

	key := fmt.Sprintf("%s:%d", serverInfo.IP, serverInfo.HTTPSPort)

	existing, exists := dc.foundServers[key]
	if !exists {
		// Nuevo servidor encontrado
		dc.foundServers[key] = serverInfo
		dc.logger.Printf("Nuevo servidor encontrado: %s (%s) vía %s",
			serverInfo.ServerName, serverInfo.IP, serverInfo.Source)

		if dc.onServerFound != nil {
			go dc.onServerFound(serverInfo)
		}
	} else {
		// Actualizar servidor existente
		existing.LastSeen = time.Now()

		// Actualizar información si es más reciente
		if serverInfo.Timestamp > existing.Timestamp {
			existing.ServerName = serverInfo.ServerName
			existing.Version = serverInfo.Version
			existing.Capabilities = serverInfo.Capabilities
			existing.HTTPPort = serverInfo.HTTPPort
			existing.Timestamp = serverInfo.Timestamp
		}
	}
}

// startServerCleanup inicia la limpieza periódica de servidores obsoletos
func (dc *DiscoveryClient) startServerCleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-dc.ctx.Done():
			dc.logger.Printf("Deteniendo limpieza de servidores...")
			return
		case <-ticker.C:
			dc.cleanupObsoleteServers()
		}
	}
}

// cleanupObsoleteServers elimina servidores que no se han visto recientemente
func (dc *DiscoveryClient) cleanupObsoleteServers() {
	dc.serversMutex.Lock()
	defer dc.serversMutex.Unlock()

	now := time.Now()
	obsoleteThreshold := 60 * time.Second

	for key, server := range dc.foundServers {
		if now.Sub(server.LastSeen) > obsoleteThreshold {
			dc.logger.Printf("Servidor obsoleto eliminado: %s (%s)",
				server.ServerName, server.IP)

			delete(dc.foundServers, key)

			if dc.onServerLost != nil {
				go dc.onServerLost(key)
			}
		}
	}
}

// GetFoundServers retorna lista de servidores encontrados
func (dc *DiscoveryClient) GetFoundServers() []*ServerInfo {
	dc.serversMutex.RLock()
	defer dc.serversMutex.RUnlock()

	servers := make([]*ServerInfo, 0, len(dc.foundServers))
	for _, server := range dc.foundServers {
		// Crear copia para evitar modificaciones concurrentes
		serverCopy := *server
		servers = append(servers, &serverCopy)
	}

	return servers
}

// GetBestServer retorna el "mejor" servidor disponible
func (dc *DiscoveryClient) GetBestServer() *ServerInfo {
	servers := dc.GetFoundServers()
	if len(servers) == 0 {
		return nil
	}

	var best *ServerInfo
	for _, server := range servers {
		if best == nil {
			best = server
			continue
		}

		// Priorizar por: 1) Más reciente, 2) mDNS sobre UDP, 3) Menor latencia
		if server.LastSeen.After(best.LastSeen) {
			best = server
		} else if server.LastSeen.Equal(best.LastSeen) {
			if server.Source == "mdns" && best.Source == "udp" {
				best = server
			}
		}
	}

	return best
}

// ValidateServer valida que un servidor responda correctamente
func (dc *DiscoveryClient) ValidateServer(server *ServerInfo) error {
	if !dc.config.ServerValidation {
		return nil
	}

	// Implementar validación básica (ping HTTP)
	url := fmt.Sprintf("http://%s:%d/health", server.IP, server.HTTPPort)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("servidor no responde: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("servidor retornó código: %d", resp.StatusCode)
	}

	return nil
}

// Stop detiene el descubrimiento
func (dc *DiscoveryClient) Stop() {
	dc.logger.Printf("Deteniendo cliente de descubrimiento...")
	dc.cancel()

	// Dar tiempo para que las goroutines terminen limpiamente
	time.Sleep(2 * time.Second)
}

// DefaultClientConfig retorna configuración por defecto
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		MulticastAddr:      "224.0.0.100",
		MulticastPort:      15000,
		MDNSServiceType:    "_evaluacion._tcp",
		DiscoveryTimeout:   15 * time.Second, // Reducido para evitar timeouts largos
		EnableMDNS:         false,
		EnableUDPMulticast: true,
		ServerValidation:   true,
	}
}

// DiscoveryManager maneja múltiples intentos de descubrimiento
type DiscoveryManager struct {
	client  *DiscoveryClient
	servers []*ServerInfo
	mutex   sync.RWMutex
	logger  *log.Logger
}

// NewDiscoveryManager crea un nuevo manager de descubrimiento
func NewDiscoveryManager() *DiscoveryManager {
	config := DefaultClientConfig()
	client := NewDiscoveryClient(config)

	logger := log.New(os.Stdout, "[DISCOVERY-MANAGER] ", log.LstdFlags)

	dm := &DiscoveryManager{
		client:  client,
		servers: make([]*ServerInfo, 0),
		logger:  logger,
	}

	// Configurar callbacks
	client.SetCallbacks(dm.onServerFound, dm.onServerLost)

	return dm
}

// onServerFound callback cuando se encuentra un servidor
func (dm *DiscoveryManager) onServerFound(server *ServerInfo) {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	dm.servers = append(dm.servers, server)
	dm.logger.Printf("Servidor agregado: %s (%s:%d)",
		server.ServerName, server.IP, server.HTTPSPort)
}

// onServerLost callback cuando se pierde un servidor
func (dm *DiscoveryManager) onServerLost(serverKey string) {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	// Filtrar servidor perdido
	filtered := make([]*ServerInfo, 0)
	for _, server := range dm.servers {
		key := fmt.Sprintf("%s:%d", server.IP, server.HTTPSPort)
		if key != serverKey {
			filtered = append(filtered, server)
		}
	}
	dm.servers = filtered

	dm.logger.Printf("Servidor perdido: %s", serverKey)
}

// DiscoverServers realiza descubrimiento por tiempo limitado
func (dm *DiscoveryManager) DiscoverServers(timeout time.Duration) ([]*ServerInfo, error) {
	dm.logger.Printf("Iniciando descubrimiento por %v...", timeout)

	// Limpiar servidores anteriores
	dm.mutex.Lock()
	dm.servers = make([]*ServerInfo, 0)
	dm.mutex.Unlock()

	// Iniciar descubrimiento
	if err := dm.client.StartDiscovery(); err != nil {
		return nil, fmt.Errorf("error iniciando descubrimiento: %v", err)
	}

	// Esperar timeout
	time.Sleep(timeout)

	// Detener descubrimiento
	dm.client.Stop()

	// Retornar servidores encontrados
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	result := make([]*ServerInfo, len(dm.servers))
	copy(result, dm.servers)

	dm.logger.Printf("Descubrimiento completado. Encontrados %d servidores", len(result))
	return result, nil
}

// ConnectToBestServer intenta conectar al mejor servidor disponible
func (dm *DiscoveryManager) ConnectToBestServer(timeout time.Duration) (*ServerInfo, error) {
	servers, err := dm.DiscoverServers(timeout)
	if err != nil {
		return nil, err
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("no se encontraron servidores")
	}

	// Intentar validar servidores
	for _, server := range servers {
		if err := dm.client.ValidateServer(server); err != nil {
			dm.logger.Printf("Servidor %s falló validación: %v", server.IP, err)
			continue
		}

		dm.logger.Printf("Conectado a servidor: %s (%s:%d)",
			server.ServerName, server.IP, server.HTTPSPort)
		return server, nil
	}

	return nil, fmt.Errorf("ningún servidor pasó la validación")
}

/* func (sc *StudentClient) registerWithServer() error {
	sc.logger.Printf("Registrándose con servidor: %s", sc.serverInfo.ServerName)

	// Crear conexión UDP al servidor
	conn, err := net.DialUDP("udp", nil, sc.serverAddr)
	if err != nil {
		return fmt.Errorf("error conectando al servidor: %v", err)
	}

	sc.udpConn = conn

	// Crear mensaje de registro
	message := &ClientMessage{
		Type:      "hello",
		Action:    "join",
		ClientID:  sc.clientInfo.ClientID,
		Data:      sc.clientInfo,
		Timestamp: time.Now().Unix(),
	}

	// Enviar mensaje
	if err := sc.sendMessage(message); err != nil {
		return fmt.Errorf("error enviando mensaje de registro: %v", err)
	}

	// Esperar respuesta de confirmación
	if err := sc.waitForWelcomeResponse(); err != nil {
		return fmt.Errorf("error esperando confirmación: %v", err)
	}

	sc.connected = true
	sc.clientInfo.Status = "connected"
	sc.logger.Printf("Registrado exitosamente con el servidor")

	return nil
} */
// Reemplaza la función registerWithServer() con esta versión corregida:
func (sc *StudentClient) registerWithServer() error {
	sc.logger.Printf("Registrándose con servidor: %s", sc.serverInfo.ServerName)

	// Crear conexión UDP local para recibir respuestas
	localAddr, err := net.ResolveUDPAddr("udp", ":0") // Puerto 0 = puerto automático
	if err != nil {
		return fmt.Errorf("error resolviendo dirección local: %v", err)
	}

	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return fmt.Errorf("error creando conexión UDP: %v", err)
	}
	defer conn.Close()

	sc.udpConn = conn

	// Actualizar la información del cliente con el puerto UDP local
	localUDPAddr := conn.LocalAddr().(*net.UDPAddr)
	sc.clientInfo.IP = localUDPAddr.IP.String()

	// Crear mensaje de registro
	message := &ClientMessage{
		Type:      "hello",
		Action:    "join",
		ClientID:  sc.clientInfo.ClientID,
		Data:      sc.clientInfo,
		Timestamp: time.Now().Unix(),
	}

	// Enviar mensaje al servidor
	if err := sc.sendMessageToServer(message); err != nil {
		return fmt.Errorf("error enviando mensaje de registro: %v", err)
	}

	// Esperar respuesta de confirmación
	if err := sc.waitForWelcomeResponse(); err != nil {
		return fmt.Errorf("error esperando confirmación: %v", err)
	}

	sc.connected = true
	sc.clientInfo.Status = "connected"
	sc.logger.Printf("Registrado exitosamente con el servidor")

	return nil
}

// Agrega esta nueva función para enviar mensajes al servidor:
func (sc *StudentClient) sendMessageToServer(message *ClientMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error serializando mensaje: %v", err)
	}

	// Enviar al servidor usando WriteTo
	_, err = sc.udpConn.WriteToUDP(data, sc.serverAddr)
	if err != nil {
		return fmt.Errorf("error enviando mensaje: %v", err)
	}

	sc.logger.Printf("Mensaje enviado al servidor %s", sc.serverAddr.String())
	return nil
}

// waitForWelcomeResponse espera la respuesta de bienvenida del servidor
func (sc *StudentClient) waitForWelcomeResponse() error {
	buffer := make([]byte, 2048)

	// Configurar timeout
	sc.udpConn.SetReadDeadline(time.Now().Add(10 * time.Second))

	n, err := sc.udpConn.Read(buffer)
	if err != nil {
		return fmt.Errorf("error leyendo respuesta: %v", err)
	}

	var response ServerResponse
	if err := json.Unmarshal(buffer[:n], &response); err != nil {
		return fmt.Errorf("error deserializando respuesta: %v", err)
	}

	if response.Type == "welcome" {
		sc.logger.Printf("Bienvenida recibida: %s", response.Message)
		return nil
	} else if response.Type == "error" {
		return fmt.Errorf("servidor rechazó registro: %s", response.Message)
	}

	return fmt.Errorf("respuesta inesperada del servidor: %s", response.Type)
}

// sendMessage envía un mensaje al servidor
/* func (sc *StudentClient) sendMessage(message *ClientMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error serializando mensaje: %v", err)
	}

	_, err = sc.udpConn.Write(data)
	if err != nil {
		return fmt.Errorf("error enviando mensaje: %v", err)
	}

	return nil
} */

func DefaultStudentConfig() *ClientConfig {
	return &ClientConfig{
		StudentName:            "", // Se establecerá después
		StudentID:              "", // Se establecerá después
		ServerDiscoveryTimeout: 30 * time.Second,
		HeartbeatInterval:      10 * time.Second,
		MulticastAddr:          "224.0.0.100",
		MulticastPort:          15000,
		AutoReconnect:          true,
		MaxReconnectTries:      5,
	}
}

// NewStudentClient crea un nuevo cliente estudiante
func NewStudentClient(config *ClientConfig) (*StudentClient, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Generar información del cliente
	clientInfo, err := generateClientInfo(config)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error generando información del cliente: %v", err)
	}

	logger := log.New(os.Stdout, "[STUDENT-CLIENT] ", log.LstdFlags|log.Lshortfile)

	return &StudentClient{
		config:     config,
		clientInfo: clientInfo,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		maxRetries: config.MaxReconnectTries,
	}, nil
}

// generateClientInfo genera la información del cliente automáticamente
func generateClientInfo(config *ClientConfig) (*ClientInfo, error) {
	// Obtener nombre del computador
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Unknown-PC"
	}

	// Obtener IP local
	localIP, err := getLocalIP()
	if err != nil {
		return nil, fmt.Errorf("error obteniendo IP local: %v", err)
	}

	// Generar ID único del cliente
	clientID := fmt.Sprintf("%s-%d", hostname, time.Now().Unix())

	// Obtener especificaciones del computador
	specs := getComputerSpecs()

	return &ClientInfo{
		ClientID:      clientID,
		ComputerName:  hostname,
		StudentName:   config.StudentName,
		StudentID:     config.StudentID,
		IP:            localIP,
		MAC:           getMACAddress(),
		LastSeen:      time.Now(),
		Status:        "connecting",
		ComputerSpecs: specs,
	}, nil
}

// getLocalIP obtiene la IP local
func getLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// getMACAddress obtiene la dirección MAC (simplificado)
func getMACAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, interf := range interfaces {
		if interf.Flags&net.FlagUp != 0 && interf.Flags&net.FlagLoopback == 0 {
			if interf.HardwareAddr != nil {
				return interf.HardwareAddr.String()
			}
		}
	}
	return ""
}

// getComputerSpecs obtiene las especificaciones del computador
func getComputerSpecs() *ComputerSpecs {
	return &ComputerSpecs{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Memory:       getMemoryInfo(),
		Processor:    getProcessorInfo(),
	}
}

// getMemoryInfo obtiene información de memoria (simplificado)
func getMemoryInfo() string {
	// En implementación real, usar syscalls o librerías específicas del OS
	return "Info no disponible"
}

// getProcessorInfo obtiene información del procesador (simplificado)
func getProcessorInfo() string {
	// En implementación real, leer desde /proc/cpuinfo en Linux o registry en Windows
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func main() {
	fmt.Println("=== Cliente de Descubrimiento de Servidores ===")

	manager := NewDiscoveryManager()

	// Buscar servidores por 15 segundos
	server, err := manager.ConnectToBestServer(15 * time.Second)
	if err != nil {
		log.Fatalf("Error conectando a servidor: %v", err)
	}

	fmt.Printf("\n✓ Conectado exitosamente:\n")
	fmt.Printf("  Servidor: %s\n", server.ServerName)
	fmt.Printf("  IP: %s\n", server.IP)
	fmt.Printf("  Puerto HTTPS: %d\n", server.HTTPSPort)
	fmt.Printf("  Versión: %s\n", server.Version)
	fmt.Printf("  Descubierto vía: %s\n", server.Source)

	// Configuración
	config := DefaultStudentConfig()

	// Crear cliente
	client, err := NewStudentClient(config)
	if err != nil {
		log.Fatalf("Error creando cliente: %v", err)
	}

	// Construir dirección UDP del servidor usando IP y puerto UDP (HTTPSPort o el puerto UDP real si lo tienes)
	serverUDPAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", server.IP, 15000)) // Asumiendo puerto 15000 para UDP
	if err != nil {
		log.Fatalf("Error resolviendo dirección UDP del servidor: %v", err)
	}
	client.serverInfo = server
	client.serverAddr = serverUDPAddr

	if err := client.registerWithServer(); err != nil {
		log.Fatalf("Error registrándose con servidor: %v", err)
	}

	fmt.Println("\nPresiona Enter para salir...")
	fmt.Scanln()
}
