# Sistema de Información Académica EdalmavaSoft
Repositorio del Sistema de Información Académico de **Instituciones Educativas Públicas de Colombia**

El sistema ha sido diseñado para gestionar de manera integral los grupos por grado en distintas sedes de una institución educativa, así como la administración de asignaturas, docentes, cargas académicas, estudiantes y evaluaciones. Su objetivo principal es la generación de instrumentos de evaluación en diversos formatos, los cuales pueden ser respondidos en línea por los estudiantes. Las calificaciones obtenidas se almacenan de forma estructurada, lo que permite la generación de reportes detallados que pueden ser integrados posteriormente con otros sistemas de información institucional. A mediano plazo, se proyecta la evolución de esta plataforma hacia un Sistema de Información Académico (SIA) completo.

En la fase actual del desarrollo, se han generado los scripts correspondientes al esquema de base de datos en PostgreSQL, así como la especificación técnica de la API conforme al estándar OpenAPI.

## Tabla de Contenido

1. [Arquitectura general del sistema](https://github.com/edalmava/sia/blob/main/arquitectura_general.md "Arquitectura general del sistema")

```mermaid
erDiagram
    ESTUDIANTE {
        int id_estudiante PK
        varchar nombres
        varchar apellidos
        varchar documento_identidad
        date fecha_nacimiento
        varchar tipo_documento
        varchar telefono
        varchar correo_electronico
        varchar direccion
        int id_municipio FK
    }
    
    DOCENTE {
        int id_docente PK
        varchar nombres
        varchar apellidos
        varchar documento_identidad
        varchar profesion
        varchar tipo_documento
        varchar telefono
        varchar correo_electronico
    }
    
    ASIGNATURA {
        int id_asignatura PK
        varchar nombre
        int intensidad_horaria
    }
    
    GRUPO {
        int id_grupo PK
        varchar nombre
        int id_grado FK
        int id_sede FK
        int id_jornada FK
    }
    
    CARGA_ACADEMICA {
        int id_carga PK
        int id_docente FK
        int id_grupo FK
        int id_asignatura FK
        int id_anio_lectivo FK
    }
    
    PERIODO {
        int id_periodo PK
        varchar nombre
        date fecha_inicio
        date fecha_fin
        int id_anio_lectivo FK
    }
    
    EVALUACION {
        int id_evaluacion PK
        int id_carga_academica FK
        int id_periodo FK
        int id_tipo_evaluacion FK
        varchar nombre
        text descripcion
        date fecha_presentacion
        int id_logro FK
    }
    
    CALIFICACION {
        int id_calificacion PK
        int id_evaluacion FK
        int id_matricula FK
        numeric nota
        varchar observaciones
        timestamp fecha_calificacion
        int usuario_modificacion FK
        timestamp fecha_modificacion
    }
    
    MATRICULA {
        int id_matricula PK
        int id_estudiante FK
        int id_grupo FK
        date fecha_matricula
        varchar estado
        int id_anio_lectivo FK
    }
    
    ESTUDIANTE ||--o{ MATRICULA : "se matricula"
    GRUPO ||--o{ MATRICULA : "contiene"
    MATRICULA ||--o{ CALIFICACION : "recibe"
    
    DOCENTE ||--o{ CARGA_ACADEMICA : "asignado a"
    GRUPO ||--o{ CARGA_ACADEMICA : "tiene asignada"
    ASIGNATURA ||--o{ CARGA_ACADEMICA : "es parte de"
    
    CARGA_ACADEMICA ||--o{ EVALUACION : "programa"
    PERIODO ||--o{ EVALUACION : "corresponde a"
    
    EVALUACION ||--o{ CALIFICACION : "genera"
```
