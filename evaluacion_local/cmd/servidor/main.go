package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/grandcat/zeroconf"
)

// DiscoveryServer maneja tanto mDNS como UDP multicast para descubrimiento
type DiscoveryServer struct {
	config       *ServerConfig
	mdnsServer   *zeroconf.Server
	udpConn      *net.UDPConn
	classroomMgr *ClassroomManager // Nueva línea
	running      bool
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	logger       *log.Logger
	serverInfo   *ServerInfo
}

// ServerConfig contiene la configuración del servidor
type ServerConfig struct {
	ServerName         string        `json:"server_name"`
	HTTPPort           int           `json:"http_port"`
	HTTPSPort          int           `json:"https_port"`
	MulticastAddr      string        `json:"multicast_addr"`
	MulticastPort      int           `json:"multicast_port"`
	MDNSServiceType    string        `json:"mdns_service_type"`
	BroadcastInterval  time.Duration `json:"broadcast_interval"`
	EnableMDNS         bool          `json:"enable_mdns"`
	EnableUDPMulticast bool          `json:"enable_udp_multicast"`
}

// ServerInfo contiene la información que se difunde
type ServerInfo struct {
	ServerName   string   `json:"server_name"`
	Institution  string   `json:"institution"` // Nueva
	Classroom    string   `json:"classroom"`   // Nueva
	Version      string   `json:"version"`
	IP           string   `json:"ip"`
	HTTPPort     int      `json:"http_port"`
	HTTPSPort    int      `json:"https_port"`
	Timestamp    int64    `json:"timestamp"`
	ServerID     string   `json:"server_id"`
	Capabilities []string `json:"capabilities"`
	MaxStudents  int      `json:"max_students"` // Nueva
	ActiveExams  []string `json:"active_exams"` // Nueva
}

// UDPMessage estructura para mensajes UDP multicast
type UDPMessage struct {
	Type      string      `json:"type"`
	Action    string      `json:"action"`
	Data      *ServerInfo `json:"data"`
	Signature string      `json:"signature,omitempty"`
}

// NewDiscoveryServer crea una nueva instancia del servidor de descubrimiento
func NewDiscoveryServer(config *ServerConfig) (*DiscoveryServer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Obtener IP local
	localIP, err := getLocalIP()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error obteniendo IP local: %v", err)
	}

	serverInfo := &ServerInfo{
		ServerName:   config.ServerName,
		Version:      "1.0.0",
		IP:           localIP,
		HTTPPort:     config.HTTPPort,
		HTTPSPort:    config.HTTPSPort,
		ServerID:     generateServerID(),
		Capabilities: []string{"evaluation", "auth", "monitoring"},
	}

	logger := log.New(os.Stdout, "[DISCOVERY] ", log.LstdFlags|log.Lshortfile)

	return &DiscoveryServer{
		config:     config,
		running:    false,
		ctx:        ctx,
		cancel:     cancel,
		logger:     logger,
		serverInfo: serverInfo,
	}, nil
}

// Start inicia los servicios de descubrimiento
func (ds *DiscoveryServer) Start() error {
	if ds.running {
		return fmt.Errorf("servidor ya está ejecutándose")
	}

	ds.running = true
	ds.logger.Printf("Iniciando servidor de descubrimiento en IP: %s", ds.serverInfo.IP)

	// Inicializar classroom manager
	ds.classroomMgr = NewClassroomManager(ds)

	// Iniciar mDNS si está habilitado
	if ds.config.EnableMDNS {
		if err := ds.startMDNS(); err != nil {
			ds.logger.Printf("Error iniciando mDNS: %v", err)
		} else {
			ds.logger.Printf("Servicio mDNS iniciado: %s.%s", ds.config.ServerName, ds.config.MDNSServiceType)
		}
	}

	// Iniciar UDP multicast si está habilitado
	if ds.config.EnableUDPMulticast {
		if err := ds.startUDPMulticast(); err != nil {
			ds.logger.Printf("Error iniciando UDP multicast: %v", err)
		} else {
			ds.logger.Printf("Servicio UDP multicast iniciado en %s:%d",
				ds.config.MulticastAddr, ds.config.MulticastPort)
		}
	}

	// Iniciar detección de estudiantes
	if err := ds.classroomMgr.Start(); err != nil {
		ds.logger.Printf("Error iniciando classroom manager: %v", err)
	} else {
		ds.logger.Printf("Sistema de detección de estudiantes iniciado")
	}

	return nil
}

// startMDNS inicia el servicio mDNS
func (ds *DiscoveryServer) startMDNS() error {
	// Crear metadatos TXT para el servicio mDNS
	txtRecords := []string{
		fmt.Sprintf("version=%s", ds.serverInfo.Version),
		fmt.Sprintf("server_id=%s", ds.serverInfo.ServerID),
		fmt.Sprintf("http_port=%d", ds.config.HTTPPort),
		fmt.Sprintf("https_port=%d", ds.config.HTTPSPort),
		fmt.Sprintf("capabilities=%s", "evaluation,auth,monitoring"),
	}

	server, err := zeroconf.Register(
		ds.config.ServerName,      // Nombre del servicio
		ds.config.MDNSServiceType, // Tipo de servicio (ej: "_evaluacion._tcp")
		"local.",                  // Dominio
		ds.config.HTTPSPort,       // Puerto principal (HTTPS)
		txtRecords,                // Registros TXT
		nil,                       // Interfaces (nil = todas)
	)

	if err != nil {
		return fmt.Errorf("error registrando servicio mDNS: %v", err)
	}

	ds.mdnsServer = server
	ds.logger.Printf("Servicio mDNS registrado exitosamente")
	return nil
}

// startUDPMulticast inicia el broadcasting UDP multicast
func (ds *DiscoveryServer) startUDPMulticast() error {
	// Configurar dirección multicast
	addr, err := net.ResolveUDPAddr("udp",
		fmt.Sprintf("%s:%d", ds.config.MulticastAddr, ds.config.MulticastPort))
	if err != nil {
		return fmt.Errorf("error resolviendo dirección multicast: %v", err)
	}

	// Crear conexión UDP
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("error creando conexión UDP: %v", err)
	}

	ds.udpConn = conn

	// Iniciar goroutine para broadcast periódico
	ds.wg.Add(1)
	go ds.udpBroadcastLoop()

	return nil
}

// udpBroadcastLoop maneja el envío periódico de mensajes multicast
func (ds *DiscoveryServer) udpBroadcastLoop() {
	defer ds.wg.Done()

	ticker := time.NewTicker(ds.config.BroadcastInterval)
	defer ticker.Stop()

	// Enviar mensaje inicial inmediatamente
	ds.sendUDPBroadcast()

	for {
		select {
		case <-ds.ctx.Done():
			ds.logger.Printf("Deteniendo broadcast UDP multicast")
			return
		case <-ticker.C:
			ds.sendUDPBroadcast()
		}
	}
}

// sendUDPBroadcast envía un mensaje de descubrimiento por UDP multicast
func (ds *DiscoveryServer) sendUDPBroadcast() {
	// Actualizar timestamp
	ds.serverInfo.Timestamp = time.Now().Unix()

	// Crear mensaje
	message := &UDPMessage{
		Type:   "broadcast",
		Action: "hello",
		Data:   ds.serverInfo,
	}

	// Serializar a JSON
	data, err := json.Marshal(message)
	if err != nil {
		ds.logger.Printf("Error serializando mensaje UDP: %v", err)
		return
	}

	// Enviar mensaje
	_, err = ds.udpConn.Write(data)
	if err != nil {
		ds.logger.Printf("Error enviando broadcast UDP: %v", err)
		return
	}

	ds.logger.Printf("Broadcast UDP enviado: %s", ds.serverInfo.ServerName)
}

// Stop detiene todos los servicios de descubrimiento
func (ds *DiscoveryServer) Stop() error {
	if !ds.running {
		return nil
	}

	ds.logger.Printf("Deteniendo servidor de descubrimiento...")
	ds.running = false

	// Cancelar contexto para detener goroutines
	ds.cancel()

	// Detener classroom manager
	if ds.classroomMgr != nil {
		ds.classroomMgr.Stop()
		ds.logger.Printf("Classroom manager detenido")
	}

	// Detener mDNS
	if ds.mdnsServer != nil {
		ds.mdnsServer.Shutdown()
		ds.logger.Printf("Servicio mDNS detenido")
	}

	// Cerrar conexión UDP
	if ds.udpConn != nil {
		ds.udpConn.Close()
		ds.logger.Printf("Conexión UDP cerrada")
	}

	// Esperar que terminen las goroutines
	ds.wg.Wait()

	ds.logger.Printf("Servidor de descubrimiento detenido completamente")
	return nil
}

// GetServerInfo retorna la información actual del servidor
func (ds *DiscoveryServer) GetServerInfo() *ServerInfo {
	ds.serverInfo.Timestamp = time.Now().Unix()
	return ds.serverInfo
}

// UpdateServerInfo actualiza la información del servidor
func (ds *DiscoveryServer) UpdateServerInfo(info *ServerInfo) {
	ds.serverInfo = info
	ds.logger.Printf("Información del servidor actualizada")
}

// getLocalIP obtiene la IP local principal
func getLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// generateServerID genera un ID único para el servidor
func generateServerID() string {
	hostname, _ := os.Hostname()
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s-%d", hostname, timestamp)
}

// DefaultConfig retorna una configuración por defecto
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		ServerName:         "Servidor Evaluaciones EdalmavaSoft",
		HTTPPort:           8080,
		HTTPSPort:          8443,
		MulticastAddr:      "224.0.0.100",
		MulticastPort:      15000,
		MDNSServiceType:    "_evaluacion._tcp",
		BroadcastInterval:  5 * time.Second,
		EnableMDNS:         true,
		EnableUDPMulticast: true,
	}
}

// LoadConfig carga configuración desde archivo JSON
func LoadConfig(filename string) (*ServerConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error leyendo archivo de configuración: %v", err)
	}

	config := &ServerConfig{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("error parseando configuración JSON: %v", err)
	}

	return config, nil
}

// SaveConfig guarda configuración a archivo JSON
func SaveConfig(config *ServerConfig, filename string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializando configuración: %v", err)
	}

	return os.WriteFile(filename, data, 0644)
}

func (ds *DiscoveryServer) GetConnectedStudents() map[string]*ClientInfo {
	if ds.classroomMgr == nil {
		return make(map[string]*ClientInfo)
	}
	return ds.classroomMgr.GetConnectedClients()
}

func (ds *DiscoveryServer) GetStudentCount() int {
	if ds.classroomMgr == nil {
		return 0
	}
	return ds.classroomMgr.GetClientCount()
}

func (ds *DiscoveryServer) GetStudentsInExam() []*ClientInfo {
	if ds.classroomMgr == nil {
		return []*ClientInfo{}
	}
	return ds.classroomMgr.GetClientsByStatus("in_exam")
}

// Función para mostrar dashboard de la sala
func (ds *DiscoveryServer) ShowClassroomDashboard() {
	students := ds.GetConnectedStudents()
	inExam := ds.GetStudentsInExam()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("DASHBOARD - %s\n", ds.config.ServerName)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Estudiantes conectados: %d\n", len(students))
	fmt.Printf("En examen: %d\n", len(inExam))
	fmt.Printf("Disponibles: %d\n", len(students)-len(inExam))

	if len(students) > 0 {
		fmt.Println("\nESTUDIANTES CONECTADOS:")
		fmt.Println(strings.Repeat("-", 60))
		for _, student := range students {
			status := "📗" // Disponible
			if student.Status == "in_exam" {
				status = "📝" // En examen
			} else if student.Status == "idle" {
				status = "💤" // Inactivo
			}

			studentName := student.StudentName
			if studentName == "" {
				studentName = "Sin identificar"
			}

			fmt.Printf("%s %s (%s) - %s - IP: %s\n",
				status, student.ComputerName, studentName, student.Status, student.IP)
		}
	}
	fmt.Println(strings.Repeat("=", 60))
}

func main() {
	// Cargar o crear configuración
	config := DefaultConfig()

	// Intentar cargar configuración desde archivo
	if configFile := os.Getenv("CONFIG_FILE"); configFile != "" {
		if loadedConfig, err := LoadConfig(configFile); err == nil {
			config = loadedConfig
			log.Printf("Configuración cargada desde: %s", configFile)
		} else {
			log.Printf("Error cargando configuración: %v, usando defaults", err)
		}
	}

	// Crear servidor de descubrimiento
	server, err := NewDiscoveryServer(config)
	if err != nil {
		log.Fatalf("Error creando servidor: %v", err)
	}

	// Configurar manejo de señales para shutdown graceful
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Iniciar servidor
	if err := server.Start(); err != nil {
		log.Fatalf("Error iniciando servidor: %v", err)
	}

	log.Printf("Servidor de descubrimiento iniciado. Presiona Ctrl+C para detener.")

	// Mostrar información del servidor cada 30 segundos
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-server.ctx.Done():
				return
			case <-ticker.C:
				info := server.GetServerInfo()
				log.Printf("Estado: Nombre=%s, IP=%s, HTTPS=%d, Clientes potenciales escuchando...",
					info.ServerName, info.IP, info.HTTPSPort)
				server.ShowClassroomDashboard() // Mostrar dashboard de la sala
			}
		}
	}()

	// Esperar señal de terminación
	<-sigChan

	// Shutdown graceful
	log.Printf("Recibida señal de terminación, deteniendo servidor...")
	if err := server.Stop(); err != nil {
		log.Printf("Error durante shutdown: %v", err)
	}

	log.Printf("Servidor detenido exitosamente")
}
