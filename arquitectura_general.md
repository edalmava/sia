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
