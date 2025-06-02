# 🧱 Arquitectura General del Sistema

Este sistema de evaluación en red local está diseñado para funcionar completamente sin conexión a Internet, empleando una arquitectura cliente-servidor sobre una red LAN dinámica. Está enfocado en ambientes educativos donde se requiere control, autenticación y seguridad en la ejecución de evaluaciones digitales.

## Componentes Principales

### Servidor del profesor

* **Lenguaje**: Go o Python (Django/FastAPI) / Alternativa: .NET Core con C#
* **Funcionalidad**:
  - Servicio de Windows que actúa como núcleo del sistema.
  - Difunde periódicamente su presencia en la red mediante UDP multicast.
  - Expone una API segura mediante HTTPS con autenticación mutua TLS (mTLS).
  - Administra la generación, distribución y evaluación automática de pruebas.
  - Proporciona una interfaz gráfica de monitoreo y control en tiempo real.

 ### Cliente del Estudiante

* **Lenguaje**: Go o Python
* **Funcionalidad**:
  - Servicio de Windows que se ejecuta en cada equipo estudiantil.
  - Detecta automáticamente al servidor mediante escucha de mensajes multicast UDP.
  - Establece una conexión segura autenticada utilizando un certificado digital único emitido por el servidor.
  - Recibe, muestra y responde la evaluación.
  - Funciona en modo quiosco, con protección frente a combinaciones de teclas o cambios de ventana.
  - Guarda respuestas localmente en caso de desconexión y las sincroniza posteriormente.
 
## 🔄 Flujo de Operación Simplificado

1. Inicio del servidor
  - El equipo del profesor inicia el servicio.
  - Se difunde la dirección IP y puerto del servidor cada 30 segundos mediante multicast UDP.
2. Descubrimiento del Servidor
  - Los servicios cliente en los equipos de los estudiantes escuchan los mensajes de difusión.
  - Al detectar el servidor, intentan iniciar una conexión segura (TLS).
3. Autenticación mutua
  - El cliente se presenta con su certificado digital.
  - El servidor valida la autenticidad del certificado (mTLS).
  - Si el cliente no está registrado, el docente puede aprobar manualmente su ingreso desde la interfaz.
4. Distribución de Evaluación
  - Una vez autenticado, el servidor envía la evaluación correspondiente al cliente.
  - La evaluación se muestra en una interfaz local que restringe acciones no autorizadas.
5. Resolución y Envío de Respuestas
  - El estudiante completa la prueba.
  - Las respuestas se cifran localmente y se envían al servidor de forma segura.
6. Evaluación y Resultados
  - El servidor califica automáticamente las respuestas (usando reglas definidas o NLP).
  - El docente puede visualizar resultados en tiempo real o exportarlos posteriormente.
7. Modo Offline (resiliencia)
  - Si se pierde la conexión con el servidor, el cliente almacena las respuestas cifradas.
  - Al restablecerse la comunicación, sincroniza automáticamente los datos.

![image](https://github.com/user-attachments/assets/fb12ad42-06c5-4e10-8a0e-352feb97a6de)

Este diseño permite una implementación robusta, segura y controlada de evaluaciones académicas, incluso en entornos con infraestructura limitada o redes inestables. Su arquitectura modular facilita futuras ampliaciones, como nuevas formas de preguntas, autenticación biométrica o integración con bases de datos externas.

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
