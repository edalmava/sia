package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

type UDPMessage struct {
	Type      string `json:"type"`
	Action    string `json:"action"`
	Data      string `json:"data"`
	ClientID  string `json:"client_id,omitempty"` // AÑADIDO: ID del cliente
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

const sharedSecretKey = "@Servidor-Multicasting-Descubrimiento-2025@"

// AÑADIDO: calculateSignature genera una firma HMAC-SHA256 para un payload de mensaje.
func calculateSignature(payload []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// AÑADIDO: verifyMessage firma un mensaje y lo compara con una firma recibida.
// Para verificar, se recalcula la firma del mensaje con el campo 'Signature' vacío.
func verifyMessage(msg ServerResponse, key string) bool {
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

func main() {
	stopChan := make(chan struct{})

	go broadcast(stopChan)
	listener(stopChan)
}

func generateClientID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	pid := os.Getpid()
	return fmt.Sprintf("%s_%d", hostname, pid)
}

func getMacAddress() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		// Saltar interfaces inactivas o de loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		if len(iface.HardwareAddr) == 0 {
			continue // sin dirección MAC
		}

		// Devolver la primera MAC válida encontrada
		return iface.HardwareAddr.String(), nil
	}

	return "", fmt.Errorf("no se encontró ninguna interfaz de red activa con dirección MAC")
}

func listener(stopChan chan struct{}) {
	addr, err := net.ResolveUDPAddr("udp", ":15000")
	if err != nil {
		panic(err)
	}
	fmt.Println("Dirección UDP resuelta: ", addr)

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("Escuchando mensajes en:", addr)

	for {
		buffer := make([]byte, 1024)
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println("Error leyendo de UDP:", err)
			continue
		}
		fmt.Printf("Recibido %d bytes desde %s: %s\n", n, remoteAddr, string(buffer[:n]))

		var msg ServerResponse
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			fmt.Println("Error al deserializar mensaje:", err)
			continue
		}

		if !verifyMessage(msg, sharedSecretKey) {
			fmt.Println("Firma del mensaje no válida, ignorando mensaje.")
			continue
		}

		if msg.Type != "" && msg.Action != "" {
			fmt.Printf("Mensaje recibido: %+v\n", msg)
		}

		if msg.Type == "welcome" && msg.Action == "accepted" {
			fmt.Println("Respuesta ACK recibida, deteniendo broadcast.")
			close(stopChan) // Señal para detener broadcast
			//return
			go heartbeat(remoteAddr) // AÑADIDO: Iniciar heartbeat
		}

	}
}

func broadcast(stopChan chan struct{}) {
	addr, err := net.ResolveUDPAddr("udp", "224.0.0.100:15000")
	if err != nil {
		fmt.Println("Error resolviendo dirección:", err)
		return
	}
	fmt.Println("Dirección Multicast UDP resuelta: ", addr)

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("Descubriendo servidor...")

	macAddress, err := getMacAddress()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	fmt.Println("Comenzando broadcast cada 10 segundos...")

	for {
		select {
		case <-stopChan:
			fmt.Println("Broadcast detenido.")
			return
		case <-ticker.C:
			message := &UDPMessage{
				Type:      "hello",
				Action:    "join",
				ClientID:  generateClientID(), // AÑADIDO: Generar ID único del cliente
				Data:      macAddress,
				Timestamp: time.Now().Unix(),
			}

			payload, err := json.Marshal(message)
			if err != nil {
				panic(err)
			}

			message.Signature = calculateSignature(payload, sharedSecretKey)

			data, err := json.Marshal(message)
			if err != nil {
				panic(err)
			}

			_, err = conn.Write(data)
			if err != nil {
				fmt.Println("Error enviando broadcast:", err)
			} else {
				fmt.Println("Broadcast enviado.")
			}
		}
	}
}

func heartbeat(remoteAddr *net.UDPAddr) {
	addr, err := net.ResolveUDPAddr("udp", remoteAddr.String())
	if err != nil {
		fmt.Println("Error resolviendo dirección:", err)
		return
	}
	fmt.Println("Dirección Servidor Multicast UDP resuelta: ", addr)

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	fmt.Println("Iniciando heartbeat...")

	for {
		select {
		case <-ticker.C:
			message := &UDPMessage{
				Type:      "heartbeat",
				Action:    "ping",
				ClientID:  generateClientID(),
				Data:      "",
				Timestamp: time.Now().Unix(),
			}

			payload, err := json.Marshal(message)
			if err != nil {
				panic(err)
			}

			message.Signature = calculateSignature(payload, sharedSecretKey)

			data, err := json.Marshal(message)
			if err != nil {
				panic(err)
			}

			_, err = conn.Write(data)
			if err != nil {
				fmt.Println("Error enviando heartbeat:", err)
			} else {
				fmt.Println("Heartbeat enviado.")
			}
		}
	}
}
