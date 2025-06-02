# Servicios Principales

## Servidor del profesor

* **Lenguaje**: Go o Python (Django/FastAPI) / Alternativa: .NET Core con C#
* **Funcionalidad**:
  - Difusión multicast UDP periódica
  - Autenticación de clientes vía TLS mutuo (certificados)
  - Gestión de evaluaciones (crear, enviar, recolectar)
  - Interfaz de monitoreo en tiempo real (panel docente)
  - Generación de instaladores personalizados para estudiantes
  - Almacenamiento local cifrado (AES-256)
  - Servicio en segundo plano con control desde interfaz o bandeja del sistema

 ## Cliente del Estudiante

* **Lenguaje**: Go o Python
* **Funcionalidad**:
  - Escucha en red local (UDP Multicast o mDNS)
  - Se conecta al servidor por TLS autenticado
  - Recibe evaluación, muestra GUI en modo quiosco
  - Envío de respuestas cifradas
  - Reintentos y almacenamiento offline si se pierde conexión
  - Servicio de autoinicio en segundo plano
  - Control desde UI local (WPF/Tauri/Electron)
 
## Flujo de Operación Simplificado

1. Inicio del servidor → difunde su IP y puerto.
2. Cliente escucha → detecta y conecta vía TLS.
3. Servidor valida certificado → autentica y responde.
4. Servidor envía evaluación → cliente la presenta.
5. Cliente responde → envía cifrado, firma HMAC.
6. Servidor registra y califica → muestra resultados al docente.
7. (Si hay caída de red) → cliente almacena local, reintenta.

```mermaid
flowchart TD
 subgraph subGraph0["Red LAN"]
    direction LR
        Client1["Equipo estudiante Servicio cliente (Go)"]
        Client2["Equipo estudiante Servicio cliente (Go)"]
        ClientN["..."]
        Server["Equipo profesor Servicio servidor (Django/Python)"]
  end
 subgraph Admin["Admin"]
        TeacherGUI["Interfaz profesor (web o local)"]
  end
    Client1 -- Descubrimiento mDNS o UDP --> Server
    Client2 -- Descubrimiento mDNS o UDP --> Server
    ClientN -- Descubrimiento mDNS o UDP --> Server
    Client1 -- "Autenticación mutua TLS - certificados únicos" --> Server
    Client2 -- Autenticación mutua TLS --> Server
    ClientN -- Autenticación mutua TLS --> Server
    Server -- Distribución de exámenes, gestión de sesiones --> Client1 & Client2 & ClientN
    Client1 -- Respuestas, heartbeat --> Server
    Client2 -- Respuestas, heartbeat --> Server
    ClientN -- Respuestas, heartbeat --> Server
    Server --> TeacherGUI
