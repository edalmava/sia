package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
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

// ClientInfo información del cliente
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

// Estructuras de mensajes
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
	UDPServerPort          int           `json:"udp_server_port"` // Puerto UDP del servidor
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
	ServerName    string    `json:"server_name"`
	Version       string    `json:"version"`
	IP            string    `json:"ip"`
	HTTPPort      int       `json:"http_port"`
	HTTPSPort     int       `json:"https_port"`
	UDPPort       int       `json:"udp_port"`
	UDPServerPort int       `json:"udp_server_port"` // Puerto UDP del servidor
	ServerID      string    `json:"server_id"`
	Capabilities  []string  `json:"capabilities"`
	Timestamp     int64     `json:"timestamp"`
	LastSeen      time.Time `json:"last_seen"`
	Source        string    `json:"source"` // "mdns" o "udp"
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
	if udpPort, ok := txtMap["udp_port"]; ok {
		fmt.Sscanf(udpPort, "%d", &serverInfo.UDPPort)
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
			existing.UDPPort = serverInfo.UDPPort
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
		UDPServerPort:      15000, // Puerto UDP específico del servidor
		MDNSServiceType:    "_evaluacion._tcp",
		DiscoveryTimeout:   15 * time.Second,
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

	return servers[0], nil // Retornar el primer servidor encontrado

	/*
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

		return nil, fmt.Errorf("ningún servidor pasó la validación")*/
}

// ********** STUDENT CLIENT FUNCTIONS ********** //

func DefaultStudentConfig() *ClientConfig {
	return &ClientConfig{
		StudentName:            "", // Se establecerá después
		StudentID:              "", // Se establecerá después
		ServerDiscoveryTimeout: 15 * time.Second,
		HeartbeatInterval:      10 * time.Second,
		MulticastAddr:          "224.0.0.100",
		MulticastPort:          15000,
		UDPServerPort:          15000,
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

// Start inicia el cliente estudiante
func (sc *StudentClient) Start() error {
	sc.running = true
	sc.logger.Printf("Iniciando cliente estudiante...")

	// Iniciar goroutines
	sc.wg.Add(1)
	go sc.heartbeatLoop()

	sc.wg.Add(1)
	go sc.listenForServerMessages()

	return nil
}

// Stop detiene el cliente estudiante
func (sc *StudentClient) Stop() {
	sc.logger.Printf("Deteniendo cliente estudiante...")
	sc.running = false
	sc.connected = false

	// Cancelar contexto
	sc.cancel()

	// Cerrar conexión UDP si existe
	if sc.udpConn != nil {
		sc.udpConn.Close()
	}

	// Esperar que terminen las goroutines
	sc.wg.Wait()

	sc.logger.Printf("Cliente estudiante detenido")
}

// registerWithServer registra el cliente con el servidor (versión mejorada)
func (sc *StudentClient) registerWithServer() error {
	sc.logger.Printf("Registrándose con servidor: %s (%s:%d)",
		sc.serverInfo.ServerName, sc.serverInfo.IP, sc.serverInfo.UDPPort)

	// Validar que tenemos información del servidor
	if sc.serverInfo == nil {
		return fmt.Errorf("información del servidor no disponible")
	}

	// Cerrar conexión UDP anterior si existe
	if sc.udpConn != nil {
		sc.udpConn.Close()
		sc.udpConn = nil
	}

	// Crear conexión UDP local para recibir respuestas
	localAddr, err := net.ResolveUDPAddr("udp", ":15000") // Puerto 0 = puerto automático
	if err != nil {
		return fmt.Errorf("error resolviendo dirección local: %v", err)
	}

	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return fmt.Errorf("error creando conexión UDP: %v", err)
	}

	sc.udpConn = conn

	// Actualizar la información del cliente con datos actuales
	localIP, err := getLocalIP()
	if err != nil {
		sc.logger.Printf("Advertencia: no se pudo obtener IP local: %v", err)
		// Continuar con la IP existente
	} else {
		sc.clientInfo.IP = localIP
	}

	// Actualizar información adicional
	sc.clientInfo.LastSeen = time.Now()
	sc.clientInfo.Status = "registering"

	// Obtener puerto local asignado
	localPort := conn.LocalAddr().(*net.UDPAddr).Port
	sc.logger.Printf("Escuchando en puerto local: %d", localPort)

	// Crear mensaje de registro con reintentos
	message := &ClientMessage{
		Type:      "hello",
		Action:    "join",
		ClientID:  sc.clientInfo.ClientID,
		Data:      sc.clientInfo,
		Timestamp: time.Now().Unix(),
	}

	// Implementar reintentos de registro
	maxAttempts := 3
	retryDelay := 2 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		sc.logger.Printf("Intento de registro %d/%d", attempt, maxAttempts)

		// Enviar mensaje al servidor
		if err := sc.sendMessageToServer(message); err != nil {
			sc.logger.Printf("Error enviando mensaje de registro (intento %d): %v", attempt, err)
			if attempt == maxAttempts {
				return fmt.Errorf("error enviando mensaje de registro después de %d intentos: %v", maxAttempts, err)
			}
			time.Sleep(retryDelay)
			continue
		}

		// Esperar respuesta de confirmación con timeout específico por intento
		welcomeTimeout := 10 * time.Second
		if err := sc.waitForWelcomeResponseWithTimeout(welcomeTimeout); err != nil {
			sc.logger.Printf("Error esperando confirmación (intento %d): %v", attempt, err)
			if attempt == maxAttempts {
				return fmt.Errorf("error esperando confirmación del servidor después de %d intentos: %v", maxAttempts, err)
			}
			time.Sleep(retryDelay)
			continue
		}

		// Si llegamos aquí, el registro fue exitoso
		sc.logger.Printf("✅ Registro exitoso con el servidor: %s", sc.serverInfo.ServerName)
		sc.connected = true
		sc.registrationTries = 0
		sc.clientInfo.Status = "connected"

		return nil
	}

	return fmt.Errorf("registro falló después de %d intentos", maxAttempts)
}

// waitForWelcomeResponseWithTimeout espera la respuesta de bienvenida con timeout configurable
func (sc *StudentClient) waitForWelcomeResponseWithTimeout(timeout time.Duration) error {
	sc.logger.Printf("Esperando respuesta de bienvenida del servidor (timeout: %v)...", timeout)

	// Configurar timeout para la respuesta
	deadline := time.Now().Add(timeout)
	sc.udpConn.SetReadDeadline(deadline)

	buffer := make([]byte, 2048)

	for time.Now().Before(deadline) {
		n, addr, err := sc.udpConn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return fmt.Errorf("timeout esperando respuesta del servidor")
			}
			return fmt.Errorf("error leyendo respuesta: %v", err)
		}

		// Verificar que la respuesta viene del servidor correcto
		if addr.IP.String() != sc.serverInfo.IP {
			sc.logger.Printf("Advertencia: respuesta de IP no esperada: %s (esperada: %s)",
				addr.IP.String(), sc.serverInfo.IP)
			continue
		}

		var response ServerResponse
		if err := json.Unmarshal(buffer[:n], &response); err != nil {
			sc.logger.Printf("Error parseando respuesta del servidor: %v", err)
			continue // Continuar esperando respuestas válidas
		}

		// Procesar respuesta
		return sc.processWelcomeResponse(&response)
	}

	return fmt.Errorf("timeout esperando respuesta válida del servidor")
}

// processWelcomeResponse procesa la respuesta de bienvenida
func (sc *StudentClient) processWelcomeResponse(response *ServerResponse) error {
	sc.logger.Printf("Respuesta del servidor: %s/%s", response.Type, response.Action)

	switch response.Type {
	case "response":
		switch response.Action {
		case "welcome":
			return sc.handleWelcomeResponse(response)
		case "error":
			return fmt.Errorf("servidor rechazó registro: %s", response.Message)
		case "busy":
			return fmt.Errorf("servidor ocupado: %s", response.Message)
		default:
			return fmt.Errorf("acción de respuesta inesperada: %s", response.Action)
		}
	case "error":
		return fmt.Errorf("error del servidor: %s", response.Message)
	default:
		return fmt.Errorf("tipo de respuesta inesperada: %s", response.Type)
	}
}

// handleWelcomeResponse maneja la respuesta de bienvenida exitosa
func (sc *StudentClient) handleWelcomeResponse(response *ServerResponse) error {
	sc.connected = true
	sc.lastHeartbeat = time.Now()
	sc.clientInfo.Status = "connected"
	sc.registrationTries = 0

	sc.logger.Printf("✅ Conectado exitosamente al servidor")

	if response.Message != "" {
		sc.logger.Printf("📨 Mensaje del servidor: %s", response.Message)
	}

	// Procesar información adicional del servidor si está disponible
	if response.ServerInfo != nil {
		sc.updateServerInfo(response.ServerInfo)
	}

	// Si el servidor asigna un examen inmediatamente
	if response.AssignedExam != "" {
		sc.clientInfo.CurrentExam = response.AssignedExam
		sc.clientInfo.ExamStartTime = time.Now().Unix()
		sc.logger.Printf("📋 Examen asignado automáticamente: %s", response.AssignedExam)
	}

	// Configurar el cliente como completamente conectado
	sc.clientInfo.LastSeen = time.Now()

	return nil
}

// updateServerInfo actualiza la información del servidor con datos más recientes
func (sc *StudentClient) updateServerInfo(newInfo *ServerInfo) {
	if newInfo == nil {
		return
	}

	// Actualizar campos si son más recientes o más completos
	if newInfo.Timestamp > sc.serverInfo.Timestamp {
		if newInfo.Version != "" {
			sc.serverInfo.Version = newInfo.Version
		}
		if len(newInfo.Capabilities) > 0 {
			sc.serverInfo.Capabilities = newInfo.Capabilities
		}
		if newInfo.HTTPPort > 0 {
			sc.serverInfo.HTTPPort = newInfo.HTTPPort
		}
		if newInfo.HTTPSPort > 0 {
			sc.serverInfo.HTTPSPort = newInfo.HTTPSPort
		}
		sc.serverInfo.Timestamp = newInfo.Timestamp
		sc.serverInfo.LastSeen = time.Now()

		sc.logger.Printf("ℹ️ Información del servidor actualizada")
	}
}

// validateRegistration valida que el registro fue exitoso
func (sc *StudentClient) validateRegistration() error {
	if !sc.connected {
		return fmt.Errorf("cliente marcado como no conectado")
	}

	if sc.udpConn == nil {
		return fmt.Errorf("conexión UDP no establecida")
	}

	if sc.serverAddr == nil {
		return fmt.Errorf("dirección del servidor no configurada")
	}

	if sc.clientInfo.Status != "connected" {
		return fmt.Errorf("estado del cliente no es 'connected': %s", sc.clientInfo.Status)
	}

	// Validación adicional: enviar ping de prueba
	if err := sc.sendTestPing(); err != nil {
		return fmt.Errorf("ping de prueba falló: %v", err)
	}

	return nil
}

// sendTestPing envía un ping de prueba al servidor para validar la conexión
func (sc *StudentClient) sendTestPing() error {
	testMessage := &ClientMessage{
		Type:      "test",
		Action:    "ping",
		ClientID:  sc.clientInfo.ClientID,
		Data:      nil, // Sin datos para ping de prueba
		Timestamp: time.Now().Unix(),
	}

	return sc.sendMessageToServer(testMessage)
}

// Continuación de las funciones faltantes del StudentClient

// waitForWelcomeResponse espera la respuesta de bienvenida del servidor
func (sc *StudentClient) waitForWelcomeResponse() error {
	sc.logger.Printf("Esperando respuesta de bienvenida del servidor...")

	// Configurar timeout para la respuesta
	sc.udpConn.SetReadDeadline(time.Now().Add(10 * time.Second))

	buffer := make([]byte, 2048)
	n, _, err := sc.udpConn.ReadFromUDP(buffer)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("timeout esperando respuesta del servidor")
		}
		return fmt.Errorf("error leyendo respuesta: %v", err)
	}

	var response ServerResponse
	if err := json.Unmarshal(buffer[:n], &response); err != nil {
		return fmt.Errorf("error parseando respuesta del servidor: %v", err)
	}

	if response.Type == "response" && response.Action == "welcome" {
		sc.connected = true
		sc.lastHeartbeat = time.Now()
		sc.clientInfo.Status = "connected"
		sc.registrationTries = 0

		sc.logger.Printf("✓ Conectado exitosamente al servidor: %s", response.Message)

		// Si el servidor asigna un examen
		if response.AssignedExam != "" {
			sc.clientInfo.CurrentExam = response.AssignedExam
			sc.clientInfo.ExamStartTime = time.Now().Unix()
			sc.logger.Printf("Examen asignado: %s", response.AssignedExam)
		}

		return nil
	}

	return fmt.Errorf("respuesta inesperada del servidor: %s", response.Message)
}

// sendMessageToServer envía un mensaje al servidor vía UDP
func (sc *StudentClient) sendMessageToServer(message *ClientMessage) error {
	if sc.serverAddr == nil {
		return fmt.Errorf("dirección del servidor no establecida")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error serializando mensaje: %v", err)
	}

	// Crear conexión temporal para envío
	conn, err := net.DialUDP("udp", nil, sc.serverAddr)
	if err != nil {
		return fmt.Errorf("error conectando al servidor: %v", err)
	}
	defer conn.Close()

	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("error enviando datos: %v", err)
	}

	return nil
}

// heartbeatLoop mantiene la conexión viva con heartbeats periódicos
func (sc *StudentClient) heartbeatLoop() {
	defer sc.wg.Done()

	ticker := time.NewTicker(sc.config.HeartbeatInterval)
	defer ticker.Stop()

	sc.logger.Printf("Iniciando loop de heartbeat cada %v", sc.config.HeartbeatInterval)

	for {
		select {
		case <-sc.ctx.Done():
			sc.logger.Printf("Deteniendo loop de heartbeat")
			return
		case <-ticker.C:
			if sc.connected {
				if err := sc.sendHeartbeat(); err != nil {
					sc.logger.Printf("Error enviando heartbeat: %v", err)
					sc.handleConnectionLost()
				}
			} else if sc.config.AutoReconnect && sc.registrationTries < sc.maxRetries {
				sc.logger.Printf("Intentando reconectar... (intento %d/%d)",
					sc.registrationTries+1, sc.maxRetries)
				if err := sc.attemptReconnection(); err != nil {
					sc.logger.Printf("Fallo en reconexión: %v", err)
				}
			}
		}
	}
}

// sendHeartbeat envía un heartbeat al servidor
func (sc *StudentClient) sendHeartbeat() error {
	// Calcular latencia de red (simplificado)
	start := time.Now()

	// Actualizar información del cliente
	sc.clientInfo.LastSeen = time.Now()
	sc.clientInfo.Status = "connected"

	message := &ClientMessage{
		Type:      "heartbeat",
		Action:    "ping",
		ClientID:  sc.clientInfo.ClientID,
		Data:      sc.clientInfo,
		Timestamp: time.Now().Unix(),
	}

	if err := sc.sendMessageToServer(message); err != nil {
		return err
	}

	sc.lastHeartbeat = time.Now()

	sc.clientInfo.NetworkLatency = int(time.Since(start).Milliseconds())

	return nil
}

// handleConnectionLost maneja la pérdida de conexión
func (sc *StudentClient) handleConnectionLost() {
	sc.logger.Printf("⚠️ Conexión con servidor perdida")
	sc.connected = false
	sc.clientInfo.Status = "disconnected"

	if sc.config.AutoReconnect && sc.registrationTries < sc.maxRetries {
		sc.logger.Printf("Configurado para auto-reconexión...")
	} else {
		sc.logger.Printf("Auto-reconexión deshabilitada o máximo de intentos alcanzado")
	}
}

// attemptReconnection intenta reconectar al servidor
func (sc *StudentClient) attemptReconnection() error {
	sc.registrationTries++

	// Backoff exponencial
	waitTime := time.Duration(math.Pow(2, float64(sc.registrationTries))) * time.Second
	time.Sleep(waitTime)

	// Intentar re-descubrir el servidor
	if err := sc.rediscoverServer(); err != nil {
		return fmt.Errorf("error re-descubriendo servidor: %v", err)
	}

	// Intentar registrarse nuevamente
	if err := sc.registerWithServer(); err != nil {
		return fmt.Errorf("error re-registrando con servidor: %v", err)
	}

	return nil
}

// rediscoverServer re-descubre el servidor
func (sc *StudentClient) rediscoverServer() error {
	dm := NewDiscoveryManager()

	server, err := dm.ConnectToBestServer(sc.config.ServerDiscoveryTimeout)
	if err != nil {
		return err
	}

	sc.serverInfo = server

	// Actualizar dirección del servidor
	sc.serverAddr, err = net.ResolveUDPAddr("udp",
		fmt.Sprintf("%s:%d", server.IP, server.UDPPort))
	if err != nil {
		return fmt.Errorf("error resolviendo dirección del servidor: %v", err)
	}

	return nil
}

// listenForServerMessages escucha mensajes del servidor
func (sc *StudentClient) listenForServerMessages() {
	defer sc.wg.Done()

	sc.logger.Printf("Iniciando listener de mensajes del servidor")

	buffer := make([]byte, 4096)

	for sc.running {
		select {
		case <-sc.ctx.Done():
			sc.logger.Printf("Deteniendo listener de mensajes")
			return
		default:
			if sc.udpConn == nil {
				time.Sleep(1 * time.Second)
				continue
			}

			// Configurar timeout de lectura
			sc.udpConn.SetReadDeadline(time.Now().Add(2 * time.Second))

			n, _, err := sc.udpConn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout normal
				}
				if sc.running {
					sc.logger.Printf("Error leyendo mensaje del servidor: %v", err)
				}
				continue
			}

			sc.processServerMessage(buffer[:n])
		}
	}
}

// processServerMessage procesa un mensaje recibido del servidor
func (sc *StudentClient) processServerMessage(data []byte) {
	var response ServerResponse
	if err := json.Unmarshal(data, &response); err != nil {
		sc.logger.Printf("Error parseando mensaje del servidor: %v", err)
		return
	}

	sc.logger.Printf("📨 Mensaje del servidor: %s/%s - %s",
		response.Type, response.Action, response.Message)

	switch response.Type {
	case "response":
		sc.handleServerResponse(&response)
	case "command":
		sc.handleServerCommand(&response)
	case "notification":
		sc.handleServerNotification(&response)
	default:
		sc.logger.Printf("Tipo de mensaje desconocido: %s", response.Type)
	}
}

// handleServerResponse maneja respuestas del servidor
func (sc *StudentClient) handleServerResponse(response *ServerResponse) {
	switch response.Action {
	case "pong":
		// Respuesta a heartbeat
		sc.lastHeartbeat = time.Now()
	case "exam_assigned":
		if response.AssignedExam != "" {
			sc.clientInfo.CurrentExam = response.AssignedExam
			sc.clientInfo.ExamStartTime = time.Now().Unix()
			sc.logger.Printf("📋 Nuevo examen asignado: %s", response.AssignedExam)
		}
	case "status_update":
		sc.logger.Printf("📊 Actualización de estado: %s", response.Message)
	}
}

// handleServerCommand maneja comandos del servidor
func (sc *StudentClient) handleServerCommand(response *ServerResponse) {
	switch response.Action {
	case "shutdown":
		sc.logger.Printf("🔴 Comando de apagado recibido del servidor")
		sc.Stop()
	case "restart_exam":
		sc.logger.Printf("🔄 Comando de reinicio de examen")
		sc.clientInfo.ExamStartTime = time.Now().Unix()
	case "lock_screen":
		sc.logger.Printf("🔒 Comando de bloqueo de pantalla")
		// Implementar bloqueo de pantalla si es necesario
	case "update_info":
		sc.logger.Printf("ℹ️ Solicitud de actualización de información")
		sc.sendInfoUpdate()
	}
}

// handleServerNotification maneja notificaciones del servidor
func (sc *StudentClient) handleServerNotification(response *ServerResponse) {
	switch response.Action {
	case "exam_time_warning":
		sc.logger.Printf("⏰ Advertencia de tiempo de examen: %s", response.Message)
	case "system_message":
		sc.logger.Printf("📢 Mensaje del sistema: %s", response.Message)
	case "maintenance":
		sc.logger.Printf("🔧 Notificación de mantenimiento: %s", response.Message)
	}
}

// sendInfoUpdate envía actualización de información al servidor
func (sc *StudentClient) sendInfoUpdate() {
	sc.clientInfo.LastSeen = time.Now()

	message := &ClientMessage{
		Type:      "update",
		Action:    "client_info",
		ClientID:  sc.clientInfo.ClientID,
		Data:      sc.clientInfo,
		Timestamp: time.Now().Unix(),
	}

	if err := sc.sendMessageToServer(message); err != nil {
		sc.logger.Printf("Error enviando actualización de info: %v", err)
	}
}

// ConnectToServer conecta el cliente a un servidor específico
func (sc *StudentClient) ConnectToServer(server *ServerInfo) error {
	sc.logger.Printf("Conectando a servidor: %s (%s:%d)",
		server.ServerName, server.IP, server.UDPPort)

	sc.serverInfo = server

	// Resolver dirección UDP del servidor
	var err error
	sc.serverAddr, err = net.ResolveUDPAddr("udp",
		fmt.Sprintf("%s:%d", server.IP, server.UDPPort))
	if err != nil {
		return fmt.Errorf("error resolviendo dirección del servidor: %v", err)
	}

	// Intentar registro
	if err := sc.registerWithServer(); err != nil {
		return fmt.Errorf("error registrando con servidor: %v", err)
	}

	return nil
}

// GetClientInfo returna la información del cliente
func (sc *StudentClient) GetClientInfo() *ClientInfo {
	return sc.clientInfo
}

// GetServerInfo returna la información del servidor conectado
func (sc *StudentClient) GetServerInfo() *ServerInfo {
	return sc.serverInfo
}

// IsConnected returna si el cliente está conectado
func (sc *StudentClient) IsConnected() bool {
	return sc.connected
}

// GetStatus returna el estado actual del cliente
func (sc *StudentClient) GetStatus() string {
	if !sc.running {
		return "stopped"
	}
	if sc.connected {
		return "connected"
	}
	return "connecting"
}

// SetStudentInfo actualiza la información del estudiante
func (sc *StudentClient) SetStudentInfo(name, id string) {
	sc.clientInfo.StudentName = name
	sc.clientInfo.StudentID = id
	sc.config.StudentName = name
	sc.config.StudentID = id
}

// SendExamResult envía resultado de examen al servidor
func (sc *StudentClient) SendExamResult(examID string, results map[string]interface{}) error {
	if !sc.connected {
		return fmt.Errorf("cliente no conectado al servidor")
	}

	// Crear estructura de datos para el resultado
	resultData := map[string]interface{}{
		"exam_id":     examID,
		"client_id":   sc.clientInfo.ClientID,
		"student_id":  sc.clientInfo.StudentID,
		"results":     results,
		"finish_time": time.Now().Unix(),
		"duration":    time.Now().Unix() - sc.clientInfo.ExamStartTime,
	}

	// Serializar resultData como JSON y ponerlo en ClientInfo
	_, err := json.Marshal(resultData)
	if err != nil {
		return fmt.Errorf("error serializando resultados: %v", err)
	}

	// Crear una copia de clientInfo con los resultados
	clientInfoCopy := *sc.clientInfo
	clientInfoCopy.Status = "exam_completed"

	message := &ClientMessage{
		Type:      "exam",
		Action:    "submit_results",
		ClientID:  sc.clientInfo.ClientID,
		Data:      &clientInfoCopy,
		Timestamp: time.Now().Unix(),
	}

	// Agregar los resultados como datos adicionales
	// (En una implementación real, podrías extender ClientMessage para incluir más campos)

	if err := sc.sendMessageToServer(message); err != nil {
		return fmt.Errorf("error enviando resultados: %v", err)
	}

	sc.logger.Printf("📤 Resultados de examen enviados: %s", examID)
	return nil
}

// ********** FUNCIONES DE UTILIDAD PRINCIPALES ********** //

// RunStudentClient función principal mejorada para ejecutar un cliente estudiante
func RunStudentClient(studentName, studentID string) error {
	fmt.Printf("🚀 Iniciando cliente estudiante...\n")
	fmt.Printf("👤 Estudiante: %s (ID: %s)\n", studentName, studentID)

	// Validar parámetros de entrada
	if studentName == "" || studentID == "" {
		return fmt.Errorf("nombre del estudiante e ID son requeridos")
	}

	// Crear configuración
	config := DefaultStudentConfig()
	config.StudentName = studentName
	config.StudentID = studentID

	// Crear cliente
	fmt.Printf("⚙️ Creando cliente estudiante...\n")
	client, err := NewStudentClient(config)
	if err != nil {
		return fmt.Errorf("error creando cliente: %v", err)
	}

	// Configurar manejo de señales para terminación limpia
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Canal para errores críticos
	errChan := make(chan error, 1)

	// Goroutine para descubrimiento y conexión
	go func() {
		if err := connectToServer(client); err != nil {
			errChan <- fmt.Errorf("error conectando: %v", err)
			return
		}

		// Iniciar cliente
		if err := client.Start(); err != nil {
			errChan <- fmt.Errorf("error iniciando cliente: %v", err)
			return
		}

		fmt.Printf("✅ Cliente iniciado exitosamente\n")
		printClientStatus(client)
	}()

	// Loop principal
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	fmt.Printf("🏃 Cliente ejecutándose... (Ctrl+C para terminar)\n")

	for {
		select {
		case <-sigChan:
			fmt.Printf("\n🛑 Señal de terminación recibida. Cerrando cliente...\n")
			return shutdownClient(client)

		case err := <-errChan:
			fmt.Printf("❌ Error crítico: %v\n", err)
			return shutdownClient(client)

		case <-ticker.C:
			// Verificar estado del cliente periódicamente
			if !client.IsConnected() {
				fmt.Printf("⚠️ Cliente desconectado. Estado: %s\n", client.GetStatus())

				// Si auto-reconexión está deshabilitada o falló, terminar
				if !config.AutoReconnect {
					return fmt.Errorf("cliente desconectado y auto-reconexión deshabilitada")
				}
			} else {
				// Mostrar estado de salud cada 30 segundos
				printHealthStatus(client)
			}
		}
	}
}

// connectToServer maneja el descubrimiento y conexión al servidor
func connectToServer(client *StudentClient) error {
	fmt.Printf("🔍 Buscando servidores disponibles...\n")

	// Descubrir servidor
	dm := NewDiscoveryManager()

	// Intentar encontrar servidor con timeout
	server, err := dm.ConnectToBestServer(client.config.DiscoveryTimeout * time.Second)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar servidor: %v", err)
	}

	fmt.Printf("📡 Servidor encontrado:\n")
	fmt.Printf("   • Nombre: %s\n", server.ServerName)
	fmt.Printf("   • IP: %s:%d\n", server.IP, server.HTTPSPort)
	fmt.Printf("   • Versión: %s\n", server.Version)
	fmt.Printf("   • Fuente: %s\n", server.Source)

	/*
		// Validar servidor antes de conectar
		fmt.Printf("🔍 Validando servidor...\n")
		if err := dm.client.ValidateServer(server); err != nil {
			return fmt.Errorf("servidor no pasó validación: %v", err)
		}
	*/

	// Conectar al servidor
	fmt.Printf("🔗 Conectando al servidor...\n")
	if err := client.ConnectToServer(server); err != nil {
		return fmt.Errorf("fallo al conectar con servidor: %v", err)
	}

	// Verificar que la conexión fue exitosa
	maxWait := 10 * time.Second
	checkInterval := 500 * time.Millisecond
	waited := time.Duration(0)

	for waited < maxWait {
		if client.IsConnected() {
			fmt.Printf("✅ Conexión establecida exitosamente\n")
			return nil
		}
		time.Sleep(checkInterval)
		waited += checkInterval
	}

	return fmt.Errorf("timeout esperando confirmación de conexión")
}

// printClientStatus imprime el estado detallado del cliente
func printClientStatus(client *StudentClient) {
	clientInfo := client.GetClientInfo()
	serverInfo := client.GetServerInfo()

	fmt.Printf("\n📊 Estado del Cliente:\n")
	fmt.Printf("   • ID Cliente: %s\n", clientInfo.ClientID)
	fmt.Printf("   • Computador: %s\n", clientInfo.ComputerName)
	fmt.Printf("   • IP Local: %s\n", clientInfo.IP)
	fmt.Printf("   • Estado: %s\n", clientInfo.Status)

	if clientInfo.MAC != "" {
		fmt.Printf("   • MAC: %s\n", clientInfo.MAC)
	}

	if clientInfo.CurrentExam != "" {
		fmt.Printf("   • Examen Actual: %s\n", clientInfo.CurrentExam)
		if clientInfo.ExamStartTime > 0 {
			startTime := time.Unix(clientInfo.ExamStartTime, 0)
			duration := time.Since(startTime)
			fmt.Printf("   • Tiempo de Examen: %v\n", duration.Round(time.Second))
		}
	}

	if serverInfo != nil {
		fmt.Printf("\n🖥️ Servidor Conectado:\n")
		fmt.Printf("   • Nombre: %s\n", serverInfo.ServerName)
		fmt.Printf("   • Dirección: %s:%d\n", serverInfo.IP, serverInfo.HTTPSPort)
		fmt.Printf("   • UDP: %d\n", serverInfo.UDPPort)
		if serverInfo.Version != "" {
			fmt.Printf("   • Versión: %s\n", serverInfo.Version)
		}
	}

	if clientInfo.ComputerSpecs != nil {
		fmt.Printf("\n💻 Especificaciones:\n")
		fmt.Printf("   • SO: %s\n", clientInfo.ComputerSpecs.OS)
		fmt.Printf("   • Arquitectura: %s\n", clientInfo.ComputerSpecs.Architecture)
		if clientInfo.ComputerSpecs.Memory != "" {
			fmt.Printf("   • Memoria: %s\n", clientInfo.ComputerSpecs.Memory)
		}
		if clientInfo.ComputerSpecs.Processor != "" {
			fmt.Printf("   • Procesador: %s\n", clientInfo.ComputerSpecs.Processor)
		}
	}

	fmt.Printf("\n")
}

// printHealthStatus imprime estado de salud resumido
func printHealthStatus(client *StudentClient) {
	status := client.GetStatus()
	clientInfo := client.GetClientInfo()

	fmt.Printf("💚 Estado: %s", status)

	if client.IsConnected() {
		fmt.Printf(" | Latencia: %dms", clientInfo.NetworkLatency)

		if clientInfo.CurrentExam != "" {
			duration := time.Since(time.Unix(clientInfo.ExamStartTime, 0))
			fmt.Printf(" | Examen: %s (%v)", clientInfo.CurrentExam, duration.Round(time.Minute))
		}
	}

	fmt.Printf("\n")
}

// shutdownClient termina el cliente limpiamente
func shutdownClient(client *StudentClient) error {
	fmt.Printf("🔄 Cerrando conexiones...\n")

	// Detener cliente
	client.Stop()

	fmt.Printf("✅ Cliente terminado exitosamente\n")
	return nil
}

// RunStudentClientWithRetry ejecuta el cliente con reintentos automáticos
func RunStudentClientWithRetry(studentName, studentID string, maxRetries int) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Printf("🔄 Intento %d/%d de ejecución del cliente\n", attempt, maxRetries)

		err := RunStudentClient(studentName, studentID)
		if err == nil {
			return nil // Éxito
		}

		lastErr = err
		fmt.Printf("❌ Intento %d falló: %v\n", attempt, err)

		if attempt < maxRetries {
			waitTime := time.Duration(attempt*5) * time.Second
			fmt.Printf("⏳ Esperando %v antes del siguiente intento...\n", waitTime)
			time.Sleep(waitTime)
		}
	}

	return fmt.Errorf("cliente falló después de %d intentos. Último error: %v", maxRetries, lastErr)
}

// RunStudentClientInteractive ejecuta el cliente en modo interactivo
func RunStudentClientInteractive() error {
	fmt.Printf("🎓 Cliente Estudiante - Modo Interactivo\n")
	fmt.Printf("=====================================\n\n")

	// Solicitar información del estudiante
	var studentName, studentID string

	fmt.Printf("Ingrese el nombre del estudiante: ")
	if _, err := fmt.Scanln(&studentName); err != nil {
		return fmt.Errorf("error leyendo nombre: %v", err)
	}

	fmt.Printf("Ingrese el ID del estudiante: ")
	if _, err := fmt.Scanln(&studentID); err != nil {
		return fmt.Errorf("error leyendo ID: %v", err)
	}

	// Ejecutar cliente
	return RunStudentClient(studentName, studentID)
}

// TestConnection prueba la conexión sin ejecutar el cliente completo
func TestConnection(timeout time.Duration) error {
	fmt.Printf("🧪 Probando conexión con servidores...\n")

	dm := NewDiscoveryManager()
	servers, err := dm.DiscoverServers(timeout)
	if err != nil {
		return fmt.Errorf("error en descubrimiento: %v", err)
	}

	if len(servers) == 0 {
		return fmt.Errorf("no se encontraron servidores")
	}

	fmt.Printf("✅ Encontrados %d servidor(es)\n", len(servers))

	for i, server := range servers {
		fmt.Printf("\n📡 Probando servidor #%d: %s\n", i+1, server.ServerName)

		if err := dm.client.ValidateServer(server); err != nil {
			fmt.Printf("❌ Validación falló: %v\n", err)
			continue
		}

		fmt.Printf("✅ Servidor validado exitosamente\n")
		fmt.Printf("   • IP: %s:%d\n", server.IP, server.HTTPSPort)
		fmt.Printf("   • UDP: %d\n", server.UDPPort)
		fmt.Printf("   • Versión: %s\n", server.Version)
	}

	return nil
}

// DiscoverAndListServers descubre y lista servidores disponibles
func DiscoverAndListServers(timeout time.Duration) error {
	fmt.Printf("🔍 Buscando servidores por %v...\n", timeout)

	dm := NewDiscoveryManager()
	servers, err := dm.DiscoverServers(timeout)
	if err != nil {
		return fmt.Errorf("error en descubrimiento: %v", err)
	}

	if len(servers) == 0 {
		fmt.Printf("❌ No se encontraron servidores\n")
		return nil
	}

	fmt.Printf("✅ Encontrados %d servidor(es):\n\n", len(servers))

	for i, server := range servers {
		fmt.Printf("📡 Servidor #%d:\n", i+1)
		fmt.Printf("   Nombre: %s\n", server.ServerName)
		fmt.Printf("   IP: %s\n", server.IP)
		fmt.Printf("   Puerto HTTP: %d\n", server.HTTPPort)
		fmt.Printf("   Puerto HTTPS: %d\n", server.HTTPSPort)
		fmt.Printf("   Puerto UDP: %d\n", server.UDPPort)
		fmt.Printf("   Versión: %s\n", server.Version)
		fmt.Printf("   ID: %s\n", server.ServerID)
		fmt.Printf("   Fuente: %s\n", server.Source)
		fmt.Printf("   Visto por última vez: %s\n", server.LastSeen.Format("15:04:05"))
		if len(server.Capabilities) > 0 {
			fmt.Printf("   Capacidades: %v\n", server.Capabilities)
		}
		fmt.Printf("\n")
	}

	return nil
}

// ********** Sección de Ejemplos **********

// EJEMPLO 1: USO BÁSICO - Cliente Simple
func ejemploBasico() {
	fmt.Println("=== EJEMPLO 1: USO BÁSICO ===")

	// Ejecutar cliente con datos del estudiante
	err := RunStudentClient("Juan Pérez", "20241234")
	if err != nil {
		log.Fatalf("Error ejecutando cliente: %v", err)
	}
}

// EJEMPLO 2: DESCUBRIMIENTO DE SERVIDORES
func ejemploDescubrimiento() {
	fmt.Println("=== EJEMPLO 2: DESCUBRIMIENTO DE SERVIDORES ===")

	// Buscar servidores disponibles por 15 segundos
	err := DiscoverAndListServers(15 * time.Second)
	if err != nil {
		log.Fatalf("Error en descubrimiento: %v", err)
	}
}

// EJEMPLO 3: CLIENTE CON CONFIGURACIÓN PERSONALIZADA
func ejemploPersonalizado() {
	fmt.Println("=== EJEMPLO 3: CONFIGURACIÓN PERSONALIZADA ===")

	// Crear configuración personalizada
	config := &ClientConfig{
		StudentName:            "María García",
		StudentID:              "20241235",
		ServerDiscoveryTimeout: 20 * time.Second,
		HeartbeatInterval:      5 * time.Second, // Heartbeat más frecuente
		MulticastAddr:          "224.0.0.100",
		MulticastPort:          15000,
		UDPServerPort:          15000,
		AutoReconnect:          true,
		MaxReconnectTries:      10,   // Más intentos de reconexión
		EnableMDNS:             true, // Habilitar mDNS
		EnableUDPMulticast:     true,
		ServerValidation:       true,
	}

	// Crear cliente con configuración personalizada
	client, err := NewStudentClient(config)
	if err != nil {
		log.Fatalf("Error creando cliente: %v", err)
	}

	// Descubrir servidor
	dm := NewDiscoveryManager()
	server, err := dm.ConnectToBestServer(config.ServerDiscoveryTimeout)
	if err != nil {
		log.Fatalf("Error encontrando servidor: %v", err)
	}

	fmt.Printf("Servidor encontrado: %s\n", server.ServerName)

	// Conectar y iniciar
	if err := client.ConnectToServer(server); err != nil {
		log.Fatalf("Error conectando: %v", err)
	}

	if err := client.Start(); err != nil {
		log.Fatalf("Error iniciando cliente: %v", err)
	}

	// Mostrar información del cliente
	info := client.GetClientInfo()
	fmt.Printf("Cliente iniciado:\n")
	fmt.Printf("  ID: %s\n", info.ClientID)
	fmt.Printf("  Nombre: %s\n", info.StudentName)
	fmt.Printf("  Computador: %s\n", info.ComputerName)
	fmt.Printf("  IP: %s\n", info.IP)
	fmt.Printf("  Estado: %s\n", client.GetStatus())

	// Mantener ejecutándose
	waitForSignal()
	client.Stop()
}

// EJEMPLO 4: CLIENTE INTERACTIVO CON MENÚ
func ejemploInteractivo() {
	fmt.Println("=== EJEMPLO 4: CLIENTE INTERACTIVO ===")

	var client *StudentClient
	var server *ServerInfo
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("\n=== MENÚ CLIENTE ESTUDIANTE ===")
		fmt.Println("1. Configurar estudiante")
		fmt.Println("2. Buscar servidores")
		fmt.Println("3. Conectar a servidor")
		fmt.Println("4. Ver estado")
		fmt.Println("5. Enviar resultado de examen")
		fmt.Println("6. Desconectar")
		fmt.Println("7. Salir")
		fmt.Print("Seleccione opción: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			client = configurarEstudiante(reader)
		case "2":
			server = buscarServidores()
		case "3":
			if client != nil && server != nil {
				conectarServidor(client, server)
			} else {
				fmt.Println("❌ Primero configure el estudiante y busque servidores")
			}
		case "4":
			mostrarEstado(client)
		case "5":
			enviarResultado(client, reader)
		case "6":
			desconectar(client)
		case "7":
			if client != nil {
				client.Stop()
			}
			fmt.Println("👋 ¡Hasta luego!")
			return
		default:
			fmt.Println("❌ Opción inválida")
		}
	}
}

// Configurar información del estudiante
func configurarEstudiante(reader *bufio.Reader) *StudentClient {
	fmt.Print("Nombre del estudiante: ")
	nombre, _ := reader.ReadString('\n')
	nombre = strings.TrimSpace(nombre)

	fmt.Print("ID del estudiante: ")
	id, _ := reader.ReadString('\n')
	id = strings.TrimSpace(id)

	config := DefaultStudentConfig()
	config.StudentName = nombre
	config.StudentID = id

	client, err := NewStudentClient(config)
	if err != nil {
		fmt.Printf("❌ Error creando cliente: %v\n", err)
		return nil
	}

	fmt.Printf("✅ Cliente configurado para %s (ID: %s)\n", nombre, id)
	return client
}

// Buscar servidores disponibles
func buscarServidores() *ServerInfo {
	fmt.Println("🔍 Buscando servidores...")

	dm := NewDiscoveryManager()
	servers, err := dm.DiscoverServers(10 * time.Second)
	if err != nil {
		fmt.Printf("❌ Error buscando servidores: %v\n", err)
		return nil
	}

	if len(servers) == 0 {
		fmt.Println("❌ No se encontraron servidores")
		return nil
	}

	fmt.Printf("✅ Encontrados %d servidor(es):\n", len(servers))
	for i, srv := range servers {
		fmt.Printf("%d. %s (%s:%d) - %s\n",
			i+1, srv.ServerName, srv.IP, srv.HTTPSPort, srv.Source)
	}

	// Retornar el mejor servidor
	return servers[0]
}

// Conectar al servidor
func conectarServidor(client *StudentClient, server *ServerInfo) {
	fmt.Printf("🔗 Conectando a %s...\n", server.ServerName)

	if err := client.ConnectToServer(server); err != nil {
		fmt.Printf("❌ Error conectando: %v\n", err)
		return
	}

	if err := client.Start(); err != nil {
		fmt.Printf("❌ Error iniciando cliente: %v\n", err)
		return
	}

	fmt.Printf("✅ Conectado exitosamente\n")
}

// Mostrar estado del cliente
func mostrarEstado(client *StudentClient) {
	if client == nil {
		fmt.Println("❌ Cliente no configurado")
		return
	}

	info := client.GetClientInfo()
	server := client.GetServerInfo()

	fmt.Println("\n📊 ESTADO DEL CLIENTE:")
	fmt.Printf("  Estado: %s\n", client.GetStatus())
	fmt.Printf("  Conectado: %v\n", client.IsConnected())
	fmt.Printf("  Estudiante: %s (ID: %s)\n", info.StudentName, info.StudentID)
	fmt.Printf("  Computador: %s\n", info.ComputerName)
	fmt.Printf("  IP: %s\n", info.IP)
	fmt.Printf("  MAC: %s\n", info.MAC)
	fmt.Printf("  Última actividad: %s\n", info.LastSeen.Format("15:04:05"))

	if server != nil {
		fmt.Printf("  Servidor: %s (%s:%d)\n", server.ServerName, server.IP, server.HTTPSPort)
	}

	if info.CurrentExam != "" {
		fmt.Printf("  Examen actual: %s\n", info.CurrentExam)
		duration := time.Now().Unix() - info.ExamStartTime
		fmt.Printf("  Tiempo transcurrido: %d minutos\n", duration/60)
	}
}

// Enviar resultado de examen (simulado)
func enviarResultado(client *StudentClient, reader *bufio.Reader) {
	if client == nil || !client.IsConnected() {
		fmt.Println("❌ Cliente no conectado")
		return
	}

	fmt.Print("ID del examen: ")
	examID, _ := reader.ReadString('\n')
	examID = strings.TrimSpace(examID)

	fmt.Print("Puntuación (0-100): ")
	score, _ := reader.ReadString('\n')
	score = strings.TrimSpace(score)

	// Crear resultados simulados
	results := map[string]interface{}{
		"score":      score,
		"answers":    []string{"A", "B", "C", "A", "D"}, // Respuestas simuladas
		"time_taken": 1800,                              // 30 minutos en segundos
		"completed":  true,
	}

	if err := client.SendExamResult(examID, results); err != nil {
		fmt.Printf("❌ Error enviando resultado: %v\n", err)
		return
	}

	fmt.Printf("✅ Resultado enviado para examen %s\n", examID)
}

// Desconectar cliente
func desconectar(client *StudentClient) {
	if client == nil {
		fmt.Println("❌ Cliente no configurado")
		return
	}

	client.Stop()
	fmt.Println("✅ Cliente desconectado")
}

// EJEMPLO 5: CLIENTE PARA APLICACIÓN GUI
func ejemploParaGUI() *ClientController {
	fmt.Println("=== EJEMPLO 5: CONTROLADOR PARA GUI ===")

	// Este sería usado desde una aplicación GUI
	controller := NewClientController()
	return controller
}

// ClientController - Controlador para aplicaciones GUI
type ClientController struct {
	client    *StudentClient
	servers   []*ServerInfo
	callbacks ClientCallbacks
	isRunning bool
}

// ClientCallbacks - Callbacks para eventos del cliente
type ClientCallbacks struct {
	OnConnected    func()
	OnDisconnected func()
	OnServerFound  func(*ServerInfo)
	OnExamAssigned func(string)
	OnMessage      func(string)
	OnError        func(error)
}

// NewClientController crea un nuevo controlador
func NewClientController() *ClientController {
	return &ClientController{
		servers: make([]*ServerInfo, 0),
	}
}

// SetCallbacks configura los callbacks
func (cc *ClientController) SetCallbacks(callbacks ClientCallbacks) {
	cc.callbacks = callbacks
}

// ConfigureStudent configura la información del estudiante
func (cc *ClientController) ConfigureStudent(name, id string) error {
	config := DefaultStudentConfig()
	config.StudentName = name
	config.StudentID = id

	client, err := NewStudentClient(config)
	if err != nil {
		if cc.callbacks.OnError != nil {
			cc.callbacks.OnError(err)
		}
		return err
	}

	cc.client = client
	return nil
}

// DiscoverServers busca servidores disponibles
func (cc *ClientController) DiscoverServers(timeout time.Duration) error {
	dm := NewDiscoveryManager()

	servers, err := dm.DiscoverServers(timeout)
	if err != nil {
		if cc.callbacks.OnError != nil {
			cc.callbacks.OnError(err)
		}
		return err
	}

	cc.servers = servers

	// Notificar servidores encontrados
	if cc.callbacks.OnServerFound != nil {
		for _, server := range servers {
			cc.callbacks.OnServerFound(server)
		}
	}

	return nil
}

// ConnectToServer conecta a un servidor específico
func (cc *ClientController) ConnectToServer(serverIndex int) error {
	if cc.client == nil {
		return fmt.Errorf("cliente no configurado")
	}

	if serverIndex < 0 || serverIndex >= len(cc.servers) {
		return fmt.Errorf("índice de servidor inválido")
	}

	server := cc.servers[serverIndex]

	if err := cc.client.ConnectToServer(server); err != nil {
		if cc.callbacks.OnError != nil {
			cc.callbacks.OnError(err)
		}
		return err
	}

	if err := cc.client.Start(); err != nil {
		if cc.callbacks.OnError != nil {
			cc.callbacks.OnError(err)
		}
		return err
	}

	cc.isRunning = true

	if cc.callbacks.OnConnected != nil {
		cc.callbacks.OnConnected()
	}

	return nil
}

// GetStatus retorna el estado actual
func (cc *ClientController) GetStatus() map[string]interface{} {
	if cc.client == nil {
		return map[string]interface{}{
			"configured": false,
			"connected":  false,
			"running":    false,
		}
	}

	info := cc.client.GetClientInfo()
	server := cc.client.GetServerInfo()

	status := map[string]interface{}{
		"configured":    true,
		"connected":     cc.client.IsConnected(),
		"running":       cc.isRunning,
		"student_name":  info.StudentName,
		"student_id":    info.StudentID,
		"computer_name": info.ComputerName,
		"ip":            info.IP,
		"current_exam":  info.CurrentExam,
	}

	if server != nil {
		status["server_name"] = server.ServerName
		status["server_ip"] = server.IP
	}

	return status
}

// Disconnect desconecta el cliente
func (cc *ClientController) Disconnect() {
	if cc.client != nil {
		cc.client.Stop()
		cc.isRunning = false

		if cc.callbacks.OnDisconnected != nil {
			cc.callbacks.OnDisconnected()
		}
	}
}

// FUNCIÓN PRINCIPAL DE EJEMPLO
func main() {
	if len(os.Args) < 2 {
		fmt.Println("Uso: cliente [opción]")
		fmt.Println("Opciones:")
		fmt.Println("  basico     - Ejemplo básico")
		fmt.Println("  descubrir  - Descubrir servidores")
		fmt.Println("  personal   - Configuración personalizada")
		fmt.Println("  interactivo - Menú interactivo")
		fmt.Println("  gui        - Ejemplo para GUI")
		return
	}

	switch os.Args[1] {
	case "basico":
		ejemploBasico()
	case "descubrir":
		ejemploDescubrimiento()
	case "personal":
		ejemploPersonalizado()
	case "interactivo":
		ejemploInteractivo()
	case "gui":
		controller := ejemploParaGUI()
		fmt.Printf("Controlador GUI creado: %+v\n", controller)
	default:
		fmt.Printf("Opción desconocida: %s\n", os.Args[1])
	}
}

// Función auxiliar para esperar señales del sistema
func waitForSignal() {
	fmt.Println("Cliente ejecutándose... Presione Ctrl+C para salir")

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	fmt.Println("\n🛑 Señal de interrupción recibida, cerrando cliente...")
}
