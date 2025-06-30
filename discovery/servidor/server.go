package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/edalmava/sia/discovery/dotenv"
)

type ClientMessage struct {
	Type      string `json:"type"`   // "hello", "heartbeat", "status_update", "goodbye"
	Action    string `json:"action"` // "join", "ping", "exam_start", "exam_end", "leave"
	Data      string `json:"data"`
	ClientID  string `json:"client_id"` // AÑADIR
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature,omitempty"` // AÑADIDO: Firma HMAC del mensaje
}

type ServerResponse struct {
	Type      string `json:"type"`   // "welcome", "pong", "command", "error"
	Action    string `json:"action"` // "accepted", "rejected", "exam_assigned", "shutdown"
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature,omitempty"` // AÑADIDO: Firma HMAC de la respuesta
}

type ServerConfig struct {
	MulticastAddress   string
	SharedSecret       string
	ResponsePort       int
	BufferSize         int
	MaxWorkers         int
	ShutdownTimeout    time.Duration
	DrainTimeout       time.Duration
	timestampTolerance time.Duration // AÑADIDO: Tolerancia de tiempo para timestamps
	MaxClients         int           // AÑADIR
	ClientTimeout      time.Duration // AÑADIR
}

type MessageJob struct {
	Message    ClientMessage
	RemoteAddr *net.UDPAddr
	Data       []byte
}

type MulticastServer struct {
	config         *ServerConfig
	conn           *net.UDPConn
	jobQueue       chan MessageJob
	workerPool     chan chan MessageJob
	workers        []Worker
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	numWorkers     int
	shutdownChan   chan struct{}
	isShuttingDown bool
	shutdownMutex  sync.RWMutex
	clientRegistry *ClientRegistry // AÑADIR
}

type Worker struct {
	id         int
	jobQueue   chan MessageJob
	workerPool chan chan MessageJob
	ctx        context.Context
	server     *MulticastServer
}

type ClientState string

const (
	ClientStateConnected    ClientState = "connected"
	ClientStateInExam       ClientState = "in_exam"
	ClientStateIdle         ClientState = "idle"
	ClientStateDisconnected ClientState = "disconnected"
)

type RegisteredClient struct {
	ID          string            `json:"id"`
	Address     *net.UDPAddr      `json:"address"`
	State       ClientState       `json:"state"`
	LastSeen    time.Time         `json:"last_seen"`
	ConnectedAt time.Time         `json:"connected_at"`
	ExamStarted *time.Time        `json:"exam_started,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"` // Para datos adicionales
}

type ClientRegistry struct {
	clients       map[string]*RegisteredClient
	mutex         sync.RWMutex
	maxClients    int
	clientTimeout time.Duration
}

func LoadConfig() *ServerConfig {
	// Valores por defecto
	config := &ServerConfig{
		MulticastAddress:   "224.0.0.100:15000",
		SharedSecret:       "@Servidor-Multicasting-Descubrimiento-2025@",
		ResponsePort:       15000,
		BufferSize:         1024,
		MaxWorkers:         12,
		ShutdownTimeout:    30 * time.Second,
		DrainTimeout:       10 * time.Second,
		timestampTolerance: 10 * time.Second, // Tolerancia de 10 segundos para timestamps
		MaxClients:         42,               // Valor por defecto
		ClientTimeout:      30 * time.Second, // Timeout por defecto para clientes
	}

	// Sobrescribir con variables de entorno
	if addr := os.Getenv("MULTICAST_ADDR"); addr != "" {
		config.MulticastAddress = addr
	}

	if secret := os.Getenv("SHARED_SECRET"); secret != "" {
		config.SharedSecret = secret
	}

	if port := os.Getenv("RESPONSE_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.ResponsePort = p
		}
	}

	if bufSize := os.Getenv("BUFFER_SIZE"); bufSize != "" {
		if size, err := strconv.Atoi(bufSize); err == nil {
			config.BufferSize = size
		}
	}

	if workers := os.Getenv("MAX_WORKERS"); workers != "" {
		if w, err := strconv.Atoi(workers); err == nil {
			config.MaxWorkers = w
		}
	}

	if timeout := os.Getenv("SHUTDOWN_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			config.ShutdownTimeout = d
		}
	}

	if timestampTolerance := os.Getenv("TIMESTAMP_TOLERANCE"); timestampTolerance != "" {
		if tol, err := time.ParseDuration(timestampTolerance); err == nil {
			config.timestampTolerance = tol
		}
	}

	if maxClients := os.Getenv("MAX_CLIENTS"); maxClients != "" {
		if mc, err := strconv.Atoi(maxClients); err == nil {
			config.MaxClients = mc
		}
	}

	if clientTimeout := os.Getenv("CLIENT_TIMEOUT"); clientTimeout != "" {
		if ct, err := time.ParseDuration(clientTimeout); err == nil {
			config.ClientTimeout = ct
		}
	}

	return config
}

// AÑADIDO: calculateSignature genera una firma HMAC-SHA256 para un payload de mensaje.
func calculateSignature(payload []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// AÑADIDO: verifyMessage firma un mensaje y lo compara con una firma recibida.
// Para verificar, se recalcula la firma del mensaje con el campo 'Signature' vacío.
func verifyMessage(msg ClientMessage, key string) bool {
	// 1. Guardar la firma recibida y limpiar el campo en el struct.
	receivedSignature := msg.Signature
	if receivedSignature == "" {
		return false // Si no hay firma, el mensaje no es válido.
	}
	msg.Signature = ""

	// 2. Convertir el mensaje (sin la firma) de nuevo a JSON para obtener el payload original.
	payload, err := json.Marshal(msg)
	if err != nil {
		fmt.Printf("Error al serializar mensaje para verificación: %v\n", err)
		return false
	}

	// 3. Calcular la firma esperada sobre ese payload.
	expectedSignature := calculateSignature(payload, key)

	// 4. Comparar de forma segura la firma recibida con la esperada.
	return hmac.Equal([]byte(receivedSignature), []byte(expectedSignature))
}

func (s *MulticastServer) verifyMessage(msg ClientMessage) bool {
	return verifyMessage(msg, s.config.SharedSecret)
}

func (s *MulticastServer) calculateSignature(payload []byte) string {
	return calculateSignature(payload, s.config.SharedSecret)
}

func calculateOptimalWorkers(numClients int) int {
	// Fórmulas basadas en experiencia con aulas de computación

	// Configuración base: 1 worker por cada 6-8 clientes
	baseWorkers := (numClients + 6) / 7

	// Mínimo absoluto para mantener responsividad
	minWorkers := 4

	// Máximo basado en cores disponibles (no exceder cores lógicos)
	// En servidores típicos de aula, limitar a 12-16 workers
	maxWorkers := 12

	workers := baseWorkers
	if workers < minWorkers {
		workers = minWorkers
	}
	if workers > maxWorkers {
		workers = maxWorkers
	}

	fmt.Printf("Cálculo: %d clientes → %d workers base → %d workers final\n",
		numClients, baseWorkers, workers)

	return workers
}

func NewClientRegistry(maxClients int, clientTimeout time.Duration) *ClientRegistry {
	return &ClientRegistry{
		clients:       make(map[string]*RegisteredClient),
		maxClients:    maxClients,
		clientTimeout: clientTimeout,
	}
}

func (cr *ClientRegistry) RegisterClient(clientID string, addr *net.UDPAddr) error {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	// Verificar límite de clientes
	if len(cr.clients) >= cr.maxClients {
		return fmt.Errorf("máximo número de clientes alcanzado (%d)", cr.maxClients)
	}

	now := time.Now()

	if existing, exists := cr.clients[clientID]; exists {
		// Cliente ya existe, actualizar
		existing.Address = addr
		existing.LastSeen = now
		existing.State = ClientStateConnected
		fmt.Printf("Cliente %s reconectado desde %s\n", clientID, addr)
	} else {
		// Nuevo cliente
		cr.clients[clientID] = &RegisteredClient{
			ID:          clientID,
			Address:     addr,
			State:       ClientStateConnected,
			LastSeen:    now,
			ConnectedAt: now,
			Metadata:    make(map[string]string),
		}
		fmt.Printf("Nuevo cliente registrado: %s desde %s\n", clientID, addr)
	}

	return nil
}

func (cr *ClientRegistry) UpdateClientState(clientID string, state ClientState) {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	if client, exists := cr.clients[clientID]; exists {
		oldState := client.State
		client.State = state
		client.LastSeen = time.Now()

		if state == ClientStateInExam && oldState != ClientStateInExam {
			now := time.Now()
			client.ExamStarted = &now
		}

		fmt.Printf("Cliente %s cambió estado: %s -> %s\n", clientID, oldState, state)
	}
}

func (cr *ClientRegistry) UpdateLastSeen(clientID string) {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()

	if client, exists := cr.clients[clientID]; exists {
		client.LastSeen = time.Now()
	}
}

func (cr *ClientRegistry) RemoveClient(clientID string) {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	if client, exists := cr.clients[clientID]; exists {
		delete(cr.clients, clientID)
		fmt.Printf("Cliente %s desconectado desde %s\n", clientID, client.Address)
	}
}

func (cr *ClientRegistry) GetActiveClients() []*RegisteredClient {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()

	var active []*RegisteredClient
	for _, client := range cr.clients {
		if client.State != ClientStateDisconnected {
			active = append(active, client)
		}
	}
	return active
}

func (cr *ClientRegistry) GetClientCount() int {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()
	return len(cr.clients)
}

func (cr *ClientRegistry) CleanupExpiredClients() int {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	var expired []string
	now := time.Now()

	for id, client := range cr.clients {
		if now.Sub(client.LastSeen) > cr.clientTimeout {
			expired = append(expired, id)
		}
	}

	for _, id := range expired {
		delete(cr.clients, id)
		fmt.Printf("Cliente %s removido por timeout\n", id)
	}

	return len(expired)
}

func NewMulticastServer(config *ServerConfig, numWorkers int) *MulticastServer {
	ctx, cancel := context.WithCancel(context.Background())

	// Ajustar tamaño de cola basado en número de clientes esperados
	// Regla: 3-5 mensajes por cliente en buffer
	queueSize := numWorkers * 25 // Aproximadamente 150 para 6 workers

	return &MulticastServer{
		config:         config, // Almacenar configuración
		jobQueue:       make(chan MessageJob, queueSize),
		workerPool:     make(chan chan MessageJob, numWorkers),
		workers:        make([]Worker, numWorkers),
		ctx:            ctx,
		cancel:         cancel,
		numWorkers:     numWorkers,
		shutdownChan:   make(chan struct{}),
		isShuttingDown: false,
		clientRegistry: NewClientRegistry(config.MaxClients, config.ClientTimeout), // AÑADIR
	}
}

func (s *MulticastServer) Start(address string) error {
	// Configurar dirección UDP multicast
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return fmt.Errorf("error resolviendo dirección: %v", err)
	}

	// Crear conexión multicast
	s.conn, err = net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("error creando conexión multicast: %v", err)
	}

	fmt.Printf("Servidor multicast iniciado en: %s\n", addr)
	fmt.Printf("Usando %d workers para procesar mensajes\n", s.numWorkers)

	// Configurar manejo de señales ANTES de iniciar goroutines
	s.setupSignalHandling()

	// Iniciar workers
	s.startWorkers()

	go s.startCleanupRoutine()

	// Iniciar dispatcher
	go s.dispatcher()

	// Bucle principal de recepción
	go s.receiveMessages()

	return nil
}

func (s *MulticastServer) startCleanupRoutine() {
	ticker := time.NewTicker(30 * time.Second) // Limpiar cada 30 segundos
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			expired := s.clientRegistry.CleanupExpiredClients()
			if expired > 0 {
				fmt.Printf("Limpieza completada: %d clientes expirados removidos\n", expired)
			}
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *MulticastServer) Wait() {
	// Esperar hasta que se reciba señal de shutdown
	<-s.shutdownChan
	fmt.Println("Servidor terminado")
}

func (s *MulticastServer) startWorkers() {
	for i := 0; i < s.numWorkers; i++ {
		worker := Worker{
			id:         i + 1,
			jobQueue:   make(chan MessageJob),
			workerPool: s.workerPool,
			ctx:        s.ctx,
			server:     s,
		}
		s.workers[i] = worker
		s.wg.Add(1)
		go worker.start(&s.wg)
	}
}

func (s *MulticastServer) dispatcher() {
	defer func() {
		fmt.Println("Dispatcher terminado")
	}()

	for {
		select {
		case job := <-s.jobQueue:
			// Buscar un worker disponible
			select {
			case workerJobQueue := <-s.workerPool:
				// Enviar trabajo al worker
				select {
				case workerJobQueue <- job:
					// Trabajo enviado exitosamente
				case <-s.ctx.Done():
					return
				}
			case <-s.ctx.Done():
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *MulticastServer) receiveMessages() {
	defer func() {
		fmt.Println("Receptor de mensajes terminado")
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.shutdownChan:
			fmt.Println("Señal de shutdown recibida en receptor")
			return
		default:
			// Verificar si estamos en shutdown
			if s.isShutdown() {
				return
			}

			// Configurar timeout para ReadFromUDP
			s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

			buffer := make([]byte, s.config.BufferSize)
			n, remoteAddr, err := s.conn.ReadFromUDP(buffer)

			if err != nil {
				// Verificar si es timeout
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout normal, continuar
				}
				// Si estamos en shutdown, errores de red son esperados
				if s.isShutdown() {
					return
				}
				fmt.Printf("Error leyendo UDP: %v\n", err)
				continue
			}

			fmt.Printf("Recibidos %d bytes desde %s: %s\n", n, remoteAddr, string(buffer[:n]))

			var message ClientMessage
			if err := json.Unmarshal(buffer[:n], &message); err != nil {
				fmt.Printf("Error deserializando mensaje: %v\n", err)
				continue
			}

			// Crear job y enviarlo a la cola
			job := MessageJob{
				Message:    message,
				RemoteAddr: remoteAddr,
				Data:       make([]byte, n),
			}
			copy(job.Data, buffer[:n])

			// Enviar a la cola de trabajos (no bloqueante)
			select {
			case s.jobQueue <- job:
				// Mensaje encolado exitosamente
			default:
				fmt.Printf("Cola de trabajos llena, descartando mensaje de %s\n", remoteAddr)
			}
		}
	}
}

func (w *Worker) start(wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("Worker %d iniciado\n", w.id)
	defer fmt.Printf("Worker %d terminado\n", w.id)

	for {
		// Registrar este worker como disponible
		select {
		case w.workerPool <- w.jobQueue:
			// Worker registrado, esperar trabajo
			select {
			case job := <-w.jobQueue:
				// Procesar el trabajo
				w.processMessage(job)
			case <-w.ctx.Done():
				return
			}
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *Worker) processMessage(job MessageJob) {
	// Primero, verificar la integridad y autenticidad del mensaje
	if !w.server.verifyMessage(job.Message) {
		fmt.Printf("Worker %d: Firma inválida o ausente para mensaje de %s. Descartando.\n", w.id, job.RemoteAddr)
		// Opcional: podrías registrar la IP para detectar posibles ataques.
		return
	}

	// Validación de timestamp (nueva)
	if !w.isValidTimestamp(job.Message.Timestamp) {
		fmt.Printf("Worker %d: Timestamp inválido para mensaje de %s. Descartando.\n", w.id, job.RemoteAddr)
		return
	}

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		fmt.Printf("Worker %d procesó mensaje tipo '%s' en %v\n", w.id, job.Message.Type, duration)
	}()

	switch job.Message.Type {
	case "hello":
		w.handleHello(job)
	case "heartbeat":
		w.handleHeartbeat(job)
	case "status_update":
		w.handleStatusUpdate(job)
	case "goodbye":
		w.handleGoodbye(job)
	default:
		w.handleUnknown(job)
	}
}

// isValidTimestamp verifica si el timestamp está dentro del margen de tolerancia
func (w *Worker) isValidTimestamp(messageTimestamp int64) bool {
	now := time.Now().Unix()
	diff := time.Duration(math.Abs(float64(now-messageTimestamp))) * time.Second
	return diff <= w.server.config.timestampTolerance
}

func (w *Worker) handleHello(job MessageJob) {
	clientID := job.Message.ClientID

	if clientID == "" {
		w.sendResponse(job.RemoteAddr, "error", "rejected", "ClientID requerido")
		return
	}

	err := w.server.clientRegistry.RegisterClient(clientID, job.RemoteAddr)
	if err != nil {
		w.sendResponse(job.RemoteAddr, "error", "rejected", err.Error())
		return
	}

	activeCount := w.server.clientRegistry.GetClientCount()
	message := fmt.Sprintf("Bienvenido al servidor. Clientes activos: %d", activeCount)

	fmt.Printf("Worker %d: Cliente %s conectado desde %s\n", w.id, clientID, job.RemoteAddr)
	w.sendResponse(job.RemoteAddr, "welcome", "accepted", message)
}

func (w *Worker) handleHeartbeat(job MessageJob) {
	clientID := job.Message.ClientID
	if clientID != "" {
		w.server.clientRegistry.UpdateLastSeen(clientID)
	}

	fmt.Printf("Worker %d: Heartbeat de cliente %s desde %s\n", w.id, clientID, job.RemoteAddr)
	w.sendResponse(job.RemoteAddr, "pong", "alive", "Servidor activo")
}

func (w *Worker) handleStatusUpdate(job MessageJob) {
	clientID := job.Message.ClientID
	if clientID != "" {
		// Parsear el estado del mensaje
		switch job.Message.Action {
		case "exam_start":
			w.server.clientRegistry.UpdateClientState(clientID, ClientStateInExam)
		case "exam_end":
			w.server.clientRegistry.UpdateClientState(clientID, ClientStateIdle)
		default:
			w.server.clientRegistry.UpdateLastSeen(clientID)
		}
	}

	fmt.Printf("Worker %d: Actualización de estado de cliente %s: %s\n", w.id, clientID, job.Message.Data)
	w.sendResponse(job.RemoteAddr, "command", "status_received", "Estado actualizado")
}

func (w *Worker) handleGoodbye(job MessageJob) {
	clientID := job.Message.ClientID
	if clientID != "" {
		w.server.clientRegistry.RemoveClient(clientID)
	}

	fmt.Printf("Worker %d: Cliente %s desconectándose desde %s\n", w.id, clientID, job.RemoteAddr)
	w.sendResponse(job.RemoteAddr, "command", "farewell", "Hasta luego")
}

func (w *Worker) handleUnknown(job MessageJob) {
	fmt.Printf("Worker %d: Tipo de mensaje desconocido '%s' de %s\n", w.id, job.Message.Type, job.RemoteAddr)
	w.sendResponse(job.RemoteAddr, "error", "unknown_type", "Tipo de mensaje no reconocido")
}

func (w *Worker) sendResponse(remoteAddr *net.UDPAddr, msgType, action, message string) {
	response := &ServerResponse{
		Type:      msgType,
		Action:    action,
		Message:   message,
		Timestamp: time.Now().Unix(),
	}

	payload, err := json.Marshal(response)
	if err != nil {
		fmt.Printf("Worker %d: Error serializando respuesta para firmar: %v\n", w.id, err)
		return
	}

	response.Signature = w.server.calculateSignature(payload)

	data, err := json.Marshal(response)
	if err != nil {
		fmt.Printf("Worker %d: Error serializando respuesta final: %v\n", w.id, err)
		return
	}

	remoteAddr.Port = w.server.config.ResponsePort // Asegurarse de que la respuesta se envíe al puerto correcto

	//_, err = conn.Write(data)
	_, err = w.server.conn.WriteToUDP(data, remoteAddr)
	if err != nil {
		fmt.Printf("Worker %d: Error enviando respuesta: %v\n", w.id, err)
		return
	}

	fmt.Printf("Worker %d: Respuesta enviada a %s\n", w.id, remoteAddr)
}

func (s *MulticastServer) isShutdown() bool {
	s.shutdownMutex.RLock()
	defer s.shutdownMutex.RUnlock()
	return s.isShuttingDown
}

func (s *MulticastServer) setShutdown(shutdown bool) {
	s.shutdownMutex.Lock()
	defer s.shutdownMutex.Unlock()
	s.isShuttingDown = shutdown
}

func (s *MulticastServer) setupSignalHandling() {
	sigChan := make(chan os.Signal, 1)

	// Registrar las señales que queremos capturar
	signal.Notify(sigChan,
		syscall.SIGINT,  // Ctrl+C
		syscall.SIGTERM, // kill command
		syscall.SIGQUIT, // Ctrl+\
		syscall.SIGHUP,  // Terminal cerrado
	)

	go func() {
		sig := <-sigChan
		fmt.Printf("\nSeñal recibida: %v\n", sig)
		fmt.Println("Iniciando shutdown graceful...")

		// Marcar que estamos en proceso de shutdown
		s.setShutdown(true)

		// Enviar señal de shutdown
		close(s.shutdownChan)

		// Iniciar proceso de shutdown con timeout
		s.gracefulShutdown(s.config.ShutdownTimeout)
	}()
}

func (s *MulticastServer) gracefulShutdown(timeout time.Duration) {
	s.sendShutdownNotification()

	fmt.Printf("Iniciando shutdown graceful con timeout de %v\n", timeout)

	// Canal para indicar que el shutdown terminó
	done := make(chan struct{})

	go func() {
		defer close(done)

		// Paso 1: Dejar de aceptar nuevos mensajes
		fmt.Println("1. Cerrando recepción de nuevos mensajes...")
		s.cancel() // Cancela el contexto para detener goroutines

		// Paso 2: Procesar mensajes pendientes en la cola
		fmt.Println("2. Procesando mensajes pendientes...")
		s.drainJobQueue()

		// Paso 3: Esperar que todos los workers terminen
		fmt.Println("3. Esperando que workers terminen...")
		s.wg.Wait()

		// Paso 4: Cerrar conexión de red
		fmt.Println("4. Cerrando conexión de red...")
		if s.conn != nil {
			s.conn.Close()
		}

		fmt.Println("Shutdown graceful completado exitosamente")
	}()

	// Esperar a que termine el shutdown o timeout
	select {
	case <-done:
		fmt.Println("Shutdown completado dentro del tiempo límite")
	case <-time.After(timeout):
		fmt.Println("Timeout de shutdown alcanzado, forzando terminación...")
		os.Exit(1)
	}
}

func (s *MulticastServer) drainJobQueue() {
	fmt.Printf("Drenando cola de trabajos (mensajes pendientes: %d)\n", len(s.jobQueue))

	// Dar tiempo limitado para procesar mensajes pendientes
	drainTimeout := time.After(s.config.DrainTimeout)

	for {
		select {
		case <-drainTimeout:
			remaining := len(s.jobQueue)
			if remaining > 0 {
				fmt.Printf("Timeout de drenado alcanzado, %d mensajes no procesados\n", remaining)
			}
			return
		default:
			if len(s.jobQueue) == 0 {
				fmt.Println("Cola de trabajos vaciada exitosamente")
				return
			}
			// Pequeña pausa para permitir que workers procesen
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (s *MulticastServer) sendShutdownNotification() {
	fmt.Println("Enviando notificación de shutdown a clientes...")

	// Crear mensaje de shutdown
	shutdownMsg := ServerResponse{
		Type:      "command",
		Action:    "shutdown",
		Message:   "Servidor cerrándose - reconectar en 30 segundos",
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(shutdownMsg)
	if err != nil {
		fmt.Printf("Error serializando mensaje de shutdown: %v\n", err)
		return
	}

	// Enviar broadcast a la dirección multicast
	multicastAddr, err := net.ResolveUDPAddr("udp", "224.0.0.100:15000")
	if err != nil {
		fmt.Printf("Error resolviendo dirección multicast: %v\n", err)
		return
	}

	conn, err := net.DialUDP("udp", nil, multicastAddr)
	if err != nil {
		fmt.Printf("Error creando conexión para shutdown: %v\n", err)
		return
	}
	defer conn.Close()

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(data)
	if err != nil {
		fmt.Printf("Error enviando notificación de shutdown: %v\n", err)
		return
	}

	fmt.Println("Notificación de shutdown enviada a clientes")
}

func main() {
	dotenv.Load(".env")
	config := LoadConfig()
	fmt.Println("Configuración cargada:")
	fmt.Printf(" - Dirección: %s\n", config.MulticastAddress)
	fmt.Printf(" - Puerto respuesta: %d\n", config.ResponsePort)
	fmt.Printf(" - Tamaño buffer: %d\n", config.BufferSize)
	fmt.Printf(" - Máx workers: %d\n", config.MaxWorkers)

	// Configuración óptima para 42 clientes
	numWorkers := calculateOptimalWorkers(42)
	fmt.Printf("Configurando servidor para 42 clientes con %d workers\n", numWorkers)

	server := NewMulticastServer(config, numWorkers)

	// Iniciar servidor
	if err := server.Start("224.0.0.100:15000"); err != nil {
		fmt.Printf("Error iniciando servidor: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Servidor iniciado. Presiona Ctrl+C para shutdown graceful")
	fmt.Println("Señales soportadas: SIGINT (Ctrl+C), SIGTERM, SIGQUIT (Ctrl+\\), SIGHUP")

	// Mostrar estadísticas periódicamente
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				count := server.clientRegistry.GetClientCount()
				fmt.Printf("=== Estadísticas: %d clientes activos ===\n", count)
			case <-server.ctx.Done():
				return
			}
		}
	}()

	// Esperar hasta que el servidor termine
	server.Wait()
}
