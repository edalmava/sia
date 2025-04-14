-- Script adaptado para PostgreSQL: Institución Educativa Pública de Colombia
-- Fecha de generación: 2025-04-13

-- =======================
-- DOMINIOS
-- =======================
CREATE DOMAIN dom_tipo_pregunta AS VARCHAR(20)
  CHECK (VALUE IN ('ABIERTA', 'OPCION_MULTIPLE', 'VERDADERO_FALSO'));

CREATE DOMAIN dom_tipo_documento AS VARCHAR(2)
  CHECK (VALUE IN ('CC', 'TI', 'RC', 'CE', 'PA'));

-- =======================
-- TABLAS PRINCIPALES
-- =======================
CREATE TABLE institucion (
    id_institucion SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    codigo_dane VARCHAR(20) UNIQUE
);

CREATE TABLE sede (
    id_sede SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    direccion VARCHAR(150),
    id_institucion INTEGER NOT NULL,
    FOREIGN KEY (id_institucion) REFERENCES institucion(id_institucion)
);

CREATE TABLE jornada (
    id_jornada SERIAL PRIMARY KEY,
    nombre VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE grado (
    id_grado SERIAL PRIMARY KEY,
    nombre VARCHAR(20) NOT NULL UNIQUE
);

CREATE TABLE grupo (
    id_grupo SERIAL PRIMARY KEY,
    nombre VARCHAR(10) NOT NULL,
    id_grado INTEGER NOT NULL,
    id_sede INTEGER NOT NULL,
    id_jornada INTEGER NOT NULL,
    CONSTRAINT unq_grupo_contexto UNIQUE (nombre, id_grado, id_sede, id_jornada)
    FOREIGN KEY (id_grado) REFERENCES grado(id_grado),
    FOREIGN KEY (id_sede) REFERENCES sede(id_sede),
    FOREIGN KEY (id_jornada) REFERENCES jornada(id_jornada)
);

CREATE TABLE estudiante (
    id_estudiante SERIAL PRIMARY KEY,
    nombres VARCHAR(100) NOT NULL,
    apellidos VARCHAR(100) NOT NULL,
    documento_identidad VARCHAR(20) NOT NULL,
    fecha_nacimiento DATE,
    tipo_documento dom_tipo_documento NOT NULL,
    CONSTRAINT unq_estudiante UNIQUE (documento_identidad, tipo_documento)
);
ALTER TABLE estudiante ADD COLUMN telefono VARCHAR(20);
ALTER TABLE estudiante ADD COLUMN correo_electronico VARCHAR(100);
ALTER TABLE estudiante ADD COLUMN direccion VARCHAR(150);
ALTER TABLE estudiante ADD COLUMN id_municipio INTEGER REFERENCES municipio(id_municipio);

CREATE TABLE docente (
    id_docente SERIAL PRIMARY KEY,
    nombres VARCHAR(100) NOT NULL,
    apellidos VARCHAR(100) NOT NULL,
    documento_identidad VARCHAR(20) NOT NULL,
    profesion VARCHAR(100),
    tipo_documento dom_tipo_documento NOT NULL,
    CONSTRAINT unq_docente UNIQUE (documento_identidad, tipo_documento)
);
ALTER TABLE docente ADD COLUMN telefono VARCHAR(20);
ALTER TABLE docente ADD COLUMN correo_electronico VARCHAR(100);

CREATE TABLE asignatura (
    id_asignatura SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL UNIQUE,
    intensidad_horaria INTEGER
);

CREATE TABLE grado_asignatura (
    id_grado INTEGER NOT NULL,
    id_asignatura INTEGER NOT NULL,
    PRIMARY KEY (id_grado, id_asignatura),
    FOREIGN KEY (id_grado) REFERENCES grado(id_grado),
    FOREIGN KEY (id_asignatura) REFERENCES asignatura(id_asignatura)
);

CREATE TABLE carga_academica (
    id_carga SERIAL PRIMARY KEY,
    id_docente INTEGER NOT NULL,
    id_grupo INTEGER NOT NULL,
    id_asignatura INTEGER NOT NULL,    
    
    FOREIGN KEY (id_docente) REFERENCES docente(id_docente),
    FOREIGN KEY (id_grupo) REFERENCES grupo(id_grupo),
    FOREIGN KEY (id_asignatura) REFERENCES asignatura(id_asignatura)
);
ALTER TABLE carga_academica ADD COLUMN id_anio_lectivo INTEGER;
ALTER TABLE carga_academica 
    ADD CONSTRAINT fk_carga_anio_lectivo 
    FOREIGN KEY (id_anio_lectivo) 
    REFERENCES anio_lectivo(id_anio_lectivo);
ALTER TABLE carga_academica ALTER COLUMN id_anio_lectivo SET NOT NULL;
ALTER TABLE carga_academica ADD CONSTRAINT unq_carga_docente 
    UNIQUE (id_docente, id_grupo, id_asignatura, id_anio_lectivo);

CREATE TABLE periodo (
    id_periodo SERIAL PRIMARY KEY,
    nombre VARCHAR(50) NOT NULL,
    fecha_inicio DATE NOT NULL,
    fecha_fin DATE NOT NULL,
    anio_lectivo INTEGER NOT NULL
);
ALTER TABLE periodo ADD CHECK (fecha_inicio < fecha_fin);

CREATE TABLE usuarios (
  id_usuario SERIAL PRIMARY KEY,
  nombre_usuario VARCHAR(50) NOT NULL UNIQUE,
  clave VARCHAR(255) NOT NULL,
  -- rol dom_rol NOT NULL,
  id_docente INTEGER,
  id_estudiante INTEGER,
  activo BOOLEAN DEFAULT TRUE,
  FOREIGN KEY (id_docente) REFERENCES docente(id_docente),
  FOREIGN KEY (id_estudiante) REFERENCES estudiante(id_estudiante)
);
ALTER TABLE usuarios ADD COLUMN salt VARCHAR(100);
-- Asegurar que el campo clave almacene hashes, no contraseñas en texto plano

-- ALTER TABLE usuarios ADD CONSTRAINT chk_usuario_rol_fk CHECK (
--    (rol = 'DOCENTE' AND id_docente IS NOT NULL AND id_estudiante IS NULL) OR
--    (rol = 'ESTUDIANTE' AND id_estudiante IS NOT NULL AND id_docente IS NULL) OR
--    (rol = 'ADMIN' AND id_docente IS NULL AND id_estudiante IS NULL)
-- );

CREATE TABLE matricula (
    id_matricula SERIAL PRIMARY KEY,
    id_estudiante INTEGER NOT NULL,
    id_grupo INTEGER NOT NULL,
    
    fecha_matricula DATE DEFAULT CURRENT_DATE,
    estado VARCHAR(20) DEFAULT 'ACTIVO' CHECK (estado IN ('ACTIVO', 'DESERTOR', 'PROMOVIDO', 'RETIRADO', 'GRADUADO')),
    FOREIGN KEY (id_estudiante) REFERENCES estudiante(id_estudiante),
    FOREIGN KEY (id_grupo) REFERENCES grupo(id_grupo)
);
ALTER TABLE matricula ADD COLUMN id_anio_lectivo INTEGER;
ALTER TABLE matricula 
    ADD CONSTRAINT fk_matricula_anio_lectivo 
    FOREIGN KEY (id_anio_lectivo) 
    REFERENCES anio_lectivo(id_anio_lectivo);
ALTER TABLE matricula ALTER COLUMN id_anio_lectivo SET NOT NULL;
ALTER TABLE matricula ADD CONSTRAINT unq_matricula_anio UNIQUE (id_estudiante, id_grupo, id_anio_lectivo);

CREATE TABLE tipo_evaluacion (
    id_tipo_evaluacion SERIAL PRIMARY KEY,
    nombre VARCHAR(50) NOT NULL UNIQUE,
    descripcion VARCHAR(255)
);

CREATE TABLE evaluacion (
    id_evaluacion SERIAL PRIMARY KEY,    
    id_carga_academica INTEGER NOT NULL,
    id_periodo INTEGER NOT NULL,    
    id_tipo_evaluacion INTEGER NOT NULL,    
    nombre VARCHAR(100) NOT NULL,
    descripcion TEXT,
    fecha_presentacion DATE,    
    FOREIGN KEY (id_periodo) REFERENCES periodo(id_periodo),
    FOREIGN KEY (id_carga_academica) REFERENCES carga_academica(id_carga),
    FOREIGN KEY (id_tipo_evaluacion) REFERENCES tipo_evaluacion(id_tipo_evaluacion)
);

CREATE TABLE calificacion (
    id_calificacion SERIAL PRIMARY KEY,    
    id_evaluacion INTEGER NOT NULL,
    id_matricula INTEGER NOT NULL,
    nota NUMERIC(4,2) CHECK (nota BETWEEN 0 AND 5),
    observaciones VARCHAR(255),
    fecha_calificacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    FOREIGN KEY (id_evaluacion) REFERENCES evaluacion(id_evaluacion),
    FOREIGN KEY (id_matricula) REFERENCES matricula(id_matricula),

    CONSTRAINT unq_evaluacion UNIQUE (id_evaluacion, id_matricula)
);

CREATE TABLE pregunta (
    id_pregunta SERIAL PRIMARY KEY,
    id_evaluacion INTEGER NOT NULL,
    enunciado VARCHAR(500) NOT NULL,
    tipo dom_tipo_pregunta,
    peso NUMERIC(3,2) DEFAULT 1.0 CHECK (peso > 0),
    FOREIGN KEY (id_evaluacion) REFERENCES evaluacion(id_evaluacion)
);

CREATE TABLE opcion_respuesta (
    id_opcion SERIAL PRIMARY KEY,
    id_pregunta INTEGER NOT NULL,
    texto VARCHAR(300) NOT NULL,
    es_correcta BOOLEAN DEFAULT FALSE,
    FOREIGN KEY (id_pregunta) REFERENCES pregunta(id_pregunta)
);

CREATE TABLE respuesta_estudiante (
    id_respuesta SERIAL PRIMARY KEY,
    id_pregunta INTEGER NOT NULL,
    id_calificacion INTEGER NOT NULL,
    respuesta VARCHAR(1000),
    puntaje_obtenido NUMERIC(4,2),
    
    id_opcion_seleccionada INTEGER,
    FOREIGN KEY (id_pregunta) REFERENCES pregunta(id_pregunta),
    FOREIGN KEY (id_calificacion) REFERENCES calificacion(id_calificacion),
    
    FOREIGN KEY (id_opcion_seleccionada) REFERENCES opcion_respuesta(id_opcion)
);

-- =======================
-- TABLA DE ASISTENCIA
-- =======================
-- Registra la asistencia diaria de los estudiantes
CREATE TABLE asistencia (
    id_asistencia SERIAL PRIMARY KEY,
    id_matricula INTEGER NOT NULL,        -- Vínculo a la matrícula del estudiante (estudiante-grupo-año)
    fecha DATE NOT NULL,                  -- Fecha para la cual se registra la asistencia
    estado VARCHAR(20) NOT NULL           -- Estado de la asistencia para esa fecha
        CHECK (estado IN ('PRESENTE',      -- El estudiante asistió
                          'AUSENTE',      -- El estudiante no asistió
                          'TARDE',        -- El estudiante llegó tarde
                          'JUSTIFICADO')), -- El estudiante tuvo una ausencia justificada
    observaciones VARCHAR(255),           -- Notas adicionales (ej. motivo de la justificación)
    fecha_registro TIMESTAMP DEFAULT CURRENT_TIMESTAMP, -- Cuándo se guardó el registro

    FOREIGN KEY (id_matricula) REFERENCES matricula(id_matricula),
    -- Asegura que solo haya un registro de asistencia por estudiante por día
    CONSTRAINT unq_asistencia_diaria UNIQUE (id_matricula, fecha)
);

-- Índices para búsquedas comunes
CREATE INDEX idx_asistencia_matricula ON asistencia(id_matricula);
CREATE INDEX idx_asistencia_fecha ON asistencia(fecha);

-- ===================================
-- TABLA DE OBSERVACIONES DE COMPORTAMIENTO
-- ===================================
-- Registra eventos o comportamientos notables de los estudiantes
CREATE TABLE observacion_comportamiento (
    id_observacion SERIAL PRIMARY KEY,
    id_matricula INTEGER NOT NULL,              -- Vínculo a la matrícula del estudiante observado
    id_docente_observador INTEGER NOT NULL,     -- Quién realizó la observación (generalmente un docente)
    id_carga_academica INTEGER,                 -- Opcional: En qué clase/asignatura ocurrió (si aplica)
    fecha_hora TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, -- Momento exacto de la observación o registro
    descripcion TEXT NOT NULL,                  -- Descripción detallada del comportamiento observado
    tipo_observacion VARCHAR(50),               -- Categoría (ej. 'CONVIVENCIA', 'ACADEMICO', 'POSITIVO', 'NEGATIVO')
    acciones_tomadas VARCHAR(500),              -- Descripción de las acciones realizadas (si hubo)

    FOREIGN KEY (id_matricula) REFERENCES matricula(id_matricula),
    FOREIGN KEY (id_docente_observador) REFERENCES docente(id_docente),
    FOREIGN KEY (id_carga_academica) REFERENCES carga_academica(id_carga) -- Puede ser NULL si no fue en una clase específica
);

CREATE TABLE anio_lectivo (
    id_anio_lectivo SERIAL PRIMARY KEY,
    anio INTEGER NOT NULL UNIQUE,
    fecha_inicio DATE NOT NULL,
    fecha_fin DATE NOT NULL,
    estado VARCHAR(20) DEFAULT 'ACTIVO' CHECK (estado IN ('ACTIVO', 'CERRADO', 'PLANEACION'))
);
-- Modificar la tabla periodo para usar esta referencia
ALTER TABLE periodo DROP COLUMN anio_lectivo;
ALTER TABLE periodo ADD COLUMN id_anio_lectivo INTEGER NOT NULL REFERENCES anio_lectivo(id_anio_lectivo);
ALTER TABLE anio_lectivo ADD CHECK (fecha_inicio < fecha_fin);

CREATE TABLE acudiente (
    id_acudiente SERIAL PRIMARY KEY,
    nombres VARCHAR(100) NOT NULL,
    apellidos VARCHAR(100) NOT NULL,
    documento_identidad VARCHAR(20) NOT NULL,
    tipo_documento dom_tipo_documento NOT NULL,
    telefono VARCHAR(20),
    direccion VARCHAR(150),
    correo_electronico VARCHAR(100),
    CONSTRAINT unq_acudiente UNIQUE (documento_identidad, tipo_documento)
);

CREATE TABLE estudiante_acudiente (
    id_estudiante INTEGER NOT NULL,
    id_acudiente INTEGER NOT NULL,
    parentesco VARCHAR(50),
    es_principal BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (id_estudiante, id_acudiente),
    FOREIGN KEY (id_estudiante) REFERENCES estudiante(id_estudiante),
    FOREIGN KEY (id_acudiente) REFERENCES acudiente(id_acudiente)
);

CREATE TABLE competencia (
    id_competencia SERIAL PRIMARY KEY,
    nombre VARCHAR(200) NOT NULL,
    descripcion TEXT,
    id_asignatura INTEGER NOT NULL,
    id_grado INTEGER NOT NULL,
    FOREIGN KEY (id_asignatura) REFERENCES asignatura(id_asignatura),
    FOREIGN KEY (id_grado) REFERENCES grado(id_grado)
);

CREATE TABLE logro (
    id_logro SERIAL PRIMARY KEY,
    descripcion TEXT NOT NULL,
    id_competencia INTEGER NOT NULL,
    id_periodo INTEGER NOT NULL,
    porcentaje NUMERIC(5,2) CHECK (porcentaje BETWEEN 0 AND 100),
    FOREIGN KEY (id_competencia) REFERENCES competencia(id_competencia),
    FOREIGN KEY (id_periodo) REFERENCES periodo(id_periodo)
);

ALTER TABLE evaluacion ADD COLUMN id_logro INTEGER REFERENCES logro(id_logro);

CREATE TABLE actividad_recuperacion (
    id_recuperacion SERIAL PRIMARY KEY,
    id_calificacion INTEGER NOT NULL,  -- Calificación original
    fecha DATE NOT NULL,
    descripcion TEXT,
    nota_recuperacion NUMERIC(4,2) CHECK (nota_recuperacion BETWEEN 0 AND 5),
    fecha_calificacion TIMESTAMP,
    FOREIGN KEY (id_calificacion) REFERENCES calificacion(id_calificacion)
);

CREATE TABLE modulo (
    id_modulo SERIAL PRIMARY KEY,
    nombre VARCHAR(50) NOT NULL UNIQUE,
    descripcion VARCHAR(255),
    codigo VARCHAR(30) NOT NULL UNIQUE
);

CREATE TABLE permiso (
    id_permiso SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    descripcion VARCHAR(255),
    codigo VARCHAR(30) NOT NULL UNIQUE,
    id_modulo INTEGER NOT NULL,
    FOREIGN KEY (id_modulo) REFERENCES modulo(id_modulo)
);

CREATE TABLE rol (
    id_rol SERIAL PRIMARY KEY,
    nombre VARCHAR(50) NOT NULL UNIQUE,
    descripcion VARCHAR(255),
    es_rol_sistema BOOLEAN DEFAULT FALSE  -- Indica si es un rol básico del sistema
);

CREATE TABLE rol_permiso (
    id_rol INTEGER NOT NULL,
    id_permiso INTEGER NOT NULL,
    PRIMARY KEY (id_rol, id_permiso),
    FOREIGN KEY (id_rol) REFERENCES rol(id_rol),
    FOREIGN KEY (id_permiso) REFERENCES permiso(id_permiso)
);

CREATE TABLE departamento (
    id_departamento SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    codigo VARCHAR(5) NOT NULL UNIQUE
);

CREATE TABLE municipio (
    id_municipio SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    codigo VARCHAR(5) NOT NULL,
    id_departamento INTEGER REFERENCES departamento(id_departamento),
    CONSTRAINT unq_municipio_depto UNIQUE (codigo, id_departamento)
);

CREATE TABLE dia_semana (
    id_dia SERIAL PRIMARY KEY,
    nombre VARCHAR(20) NOT NULL UNIQUE,
    codigo SMALLINT NOT NULL UNIQUE -- 1=Lunes, 2=Martes, etc.
);

CREATE TABLE horario (
    id_horario SERIAL PRIMARY KEY,
    id_carga_academica INTEGER NOT NULL REFERENCES carga_academica(id_carga),
    id_dia SMALLINT NOT NULL REFERENCES dia_semana(id_dia),
    hora_inicio TIME NOT NULL,
    hora_fin TIME NOT NULL,
    CONSTRAINT chk_hora CHECK (hora_fin > hora_inicio),
    CONSTRAINT unq_horario UNIQUE (id_carga_academica, id_dia, hora_inicio)
);

CREATE TABLE archivo_digital (
    id_archivo SERIAL PRIMARY KEY,
    tipo_archivo VARCHAR(50) NOT NULL, -- 'FOTO_ESTUDIANTE', 'DOCUMENTO_IDENTIDAD', etc.
    nombre_archivo VARCHAR(255) NOT NULL,
    ruta_almacenamiento VARCHAR(500) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    fecha_carga TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    id_usuario_carga INTEGER REFERENCES usuarios(id_usuario),
    entidad_relacionada VARCHAR(50) NOT NULL, -- 'ESTUDIANTE', 'DOCENTE', etc.
    id_entidad INTEGER NOT NULL -- ID del estudiante, docente, etc.
);

ALTER TABLE sede ADD COLUMN id_municipio INTEGER REFERENCES municipio(id_municipio);

ALTER TABLE usuarios ADD COLUMN id_rol INTEGER;
ALTER TABLE usuarios ADD CONSTRAINT fk_usuario_rol 
    FOREIGN KEY (id_rol) REFERENCES rol(id_rol);

ALTER TABLE calificacion ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario);
ALTER TABLE calificacion ADD COLUMN fecha_modificacion TIMESTAMP;

-- 1. Estudiante (datos personales de estudiantes)
ALTER TABLE estudiante 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- 2. Docente (datos personales de docentes)
ALTER TABLE docente 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- 3. Matricula (registro crítico de estudiantes)
ALTER TABLE matricula 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- 4. Evaluacion (información de evaluaciones)
ALTER TABLE evaluacion 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- 5. Calificacion (ya tiene algunos campos, completarlos)
ALTER TABLE calificacion 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
-- No agregamos modificación porque ya existen

-- 6. Asistencia (registros de asistencia)
ALTER TABLE asistencia 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- 7. Observacion_comportamiento (observaciones disciplinarias)
ALTER TABLE observacion_comportamiento 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- 8. Acudiente (datos de acudientes)
ALTER TABLE acudiente 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- 9. Carga_academica (asignación de docentes)
ALTER TABLE carga_academica 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- 10. Logro (definición de logros académicos)
ALTER TABLE logro 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- 11. Actividad_recuperacion (registros de recuperaciones)
ALTER TABLE actividad_recuperacion 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- 12. Usuarios (creación y modificación de usuarios)
ALTER TABLE usuarios 
ADD COLUMN usuario_creacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN usuario_modificacion INTEGER REFERENCES usuarios(id_usuario),
ADD COLUMN fecha_modificacion TIMESTAMP;

-- Índices para búsquedas comunes
CREATE INDEX idx_observacion_matricula ON observacion_comportamiento(id_matricula);
CREATE INDEX idx_observacion_docente ON observacion_comportamiento(id_docente_observador);
CREATE INDEX idx_observacion_fecha ON observacion_comportamiento(fecha_hora);
CREATE INDEX idx_observacion_carga ON observacion_comportamiento(id_carga_academica);

CREATE INDEX idx_calificacion_matricula ON calificacion(id_matricula);
CREATE INDEX idx_calificacion_evaluacion ON calificacion(id_evaluacion);
CREATE INDEX idx_respuesta_calificacion ON respuesta_estudiante(id_calificacion);
CREATE INDEX idx_respuesta_pregunta ON respuesta_estudiante(id_pregunta);
CREATE INDEX idx_matricula_estudiante ON matricula(id_estudiante);
CREATE INDEX idx_matricula_grupo ON matricula(id_grupo);

CREATE INDEX idx_carga_anio_lectivo ON carga_academica(id_anio_lectivo);
CREATE INDEX idx_matricula_anio_lectivo ON matricula(id_anio_lectivo);

CREATE INDEX idx_estudiante_documento ON estudiante(documento_identidad);
CREATE INDEX idx_docente_documento ON docente(documento_identidad);

CREATE INDEX idx_estudiante_periodo ON matriculas(id_estudiante, id_periodo);
CREATE INDEX idx_docente_asignatura ON asignaturas(id_docente);

-- Insertar roles básicos
INSERT INTO rol (nombre, descripcion, es_rol_sistema) VALUES 
('ADMIN', 'Administrador del sistema con acceso completo', TRUE),
('DOCENTE', 'Docente con acceso a gestión académica', TRUE),
('ESTUDIANTE', 'Estudiante con acceso limitado', TRUE),
('COORDINADOR', 'Coordinador académico o de disciplina', FALSE),
('SECRETARIA', 'Personal administrativo', FALSE);

-- Insertar algunos módulos
INSERT INTO modulo (nombre, descripcion, codigo) VALUES
('Administración', 'Configuración general del sistema', 'ADMIN'),
('Académico', 'Gestión de evaluaciones y calificaciones', 'ACAD'),
('Asistencia', 'Control de asistencia', 'ASIST'),
('Disciplina', 'Observaciones y procesos disciplinarios', 'DISC'),
('Reportes', 'Generación de informes', 'REPORT');

-- Insertar algunos permisos
INSERT INTO permiso (nombre, descripcion, codigo, id_modulo) VALUES
-- Permisos para administración
('Ver configuración', 'Acceso a visualizar configuración', 'ADMIN_VIEW', 1),
('Editar configuración', 'Modificar parámetros de configuración', 'ADMIN_EDIT', 1),
('Gestionar usuarios', 'Crear y modificar usuarios', 'ADMIN_USERS', 1),

-- Permisos para académico
('Ver notas', 'Visualizar calificaciones', 'ACAD_VIEW_NOTES', 2),
('Registrar notas', 'Ingresar calificaciones', 'ACAD_EDIT_NOTES', 2),
('Gestionar logros', 'Administrar logros y competencias', 'ACAD_LOGROS', 2),

-- Permisos para asistencia
('Ver asistencia', 'Visualizar registros de asistencia', 'ASIST_VIEW', 3),
('Registrar asistencia', 'Ingresar asistencia', 'ASIST_EDIT', 3),

-- Permisos para disciplina
('Ver observaciones', 'Visualizar observaciones de comportamiento', 'DISC_VIEW', 4),
('Registrar observaciones', 'Ingresar observaciones', 'DISC_EDIT', 4),

-- Permisos para reportes
('Ver reportes básicos', 'Acceso a reportes sencillos', 'REPORT_BASIC', 5),
('Generar reportes avanzados', 'Acceso a reportes complejos', 'REPORT_ADV', 5);

-- Asignar permisos a roles
-- Administrador (todos los permisos)
INSERT INTO rol_permiso (id_rol, id_permiso)
SELECT 1, id_permiso FROM permiso;

-- Docente (permisos limitados)
INSERT INTO rol_permiso (id_rol, id_permiso)
SELECT 2, id_permiso FROM permiso 
WHERE codigo IN ('ACAD_VIEW_NOTES', 'ACAD_EDIT_NOTES', 'ASIST_VIEW', 'ASIST_EDIT', 'DISC_VIEW', 'DISC_EDIT', 'REPORT_BASIC');

-- Estudiante (permisos mínimos)
INSERT INTO rol_permiso (id_rol, id_permiso)
SELECT 3, id_permiso FROM permiso 
WHERE codigo IN ('ACAD_VIEW_NOTES', 'ASIST_VIEW');

CREATE OR REPLACE FUNCTION tiene_permiso(p_id_usuario INTEGER, p_codigo_permiso VARCHAR) RETURNS BOOLEAN AS $$
DECLARE
    v_tiene BOOLEAN;
BEGIN
    SELECT EXISTS(
        SELECT 1 
        FROM usuarios u
        JOIN rol_permiso rp ON u.id_rol = rp.id_rol
        JOIN permiso p ON rp.id_permiso = p.id_permiso
        WHERE u.id_usuario = p_id_usuario
        AND p.codigo = p_codigo_permiso
    ) INTO v_tiene;
    
    RETURN v_tiene;
END;
$$ LANGUAGE plpgsql;