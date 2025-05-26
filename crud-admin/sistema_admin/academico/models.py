from django.db import models
from django.core.validators import MinValueValidator, MaxValueValidator
from django.utils import timezone
import hashlib
import os

# Dominios/Choices
TIPO_DOCUMENTO_CHOICES = [
    ('CC', 'Cédula de Ciudadanía'),
    ('TI', 'Tarjeta de Identidad'),
    ('RC', 'Registro Civil'),
    ('CE', 'Cédula de Extranjería'),
    ('PA', 'Pasaporte'),
]

TIPO_PREGUNTA_CHOICES = [
    ('ABIERTA', 'Abierta'),
    ('OPCION_MULTIPLE', 'Opción Múltiple'),
    ('VERDADERO_FALSO', 'Verdadero o Falso'),
]

ESTADO_MATRICULA_CHOICES = [
    ('ACTIVO', 'Activo'),
    ('DESERTOR', 'Desertor'),
    ('PROMOVIDO', 'Promovido'),
    ('RETIRADO', 'Retirado'),
    ('GRADUADO', 'Graduado'),
]

ESTADO_ASISTENCIA_CHOICES = [
    ('PRESENTE', 'Presente'),
    ('AUSENTE', 'Ausente'),
    ('TARDE', 'Tarde'),
    ('JUSTIFICADO', 'Justificado'),
]

ESTADO_ANIO_LECTIVO_CHOICES = [
    ('ACTIVO', 'Activo'),
    ('CERRADO', 'Cerrado'),
    ('PLANEACION', 'Planeación'),
]


class Departamento(models.Model):
    nombre = models.CharField(max_length=100)
    codigo = models.CharField(max_length=5, unique=True)

    def __str__(self):
        return self.nombre

    class Meta:
        db_table = 'departamento'
        verbose_name = "Departamento"
        verbose_name_plural = "Departamentos"


class Municipio(models.Model):
    nombre = models.CharField(max_length=100)
    codigo = models.CharField(max_length=5)
    departamento = models.ForeignKey(Departamento, on_delete=models.PROTECT, related_name='municipios')

    def __str__(self):
        return f"{self.nombre}, {self.departamento.nombre}"

    class Meta:
        db_table = 'municipio'
        verbose_name = "Municipio"
        verbose_name_plural = "Municipios"
        unique_together = ['codigo', 'departamento']


class Institucion(models.Model):
    nombre = models.CharField(max_length=100)
    codigo_dane = models.CharField(max_length=20, unique=True)

    def __str__(self):
        return self.nombre

    class Meta:
        db_table = 'institucion'
        verbose_name = "Institución"
        verbose_name_plural = "Instituciones"


class Sede(models.Model):
    nombre = models.CharField(max_length=100)
    direccion = models.CharField(max_length=150, blank=True, null=True)
    institucion = models.ForeignKey(Institucion, on_delete=models.CASCADE, related_name='sedes')
    municipio = models.ForeignKey(Municipio, on_delete=models.PROTECT, related_name='sedes', null=True)

    def __str__(self):
        return f"{self.nombre} - {self.institucion.nombre}"

    class Meta:
        db_table = 'sede'
        verbose_name = "Sede"
        verbose_name_plural = "Sedes"


class Jornada(models.Model):
    nombre = models.CharField(max_length=50, unique=True)

    def __str__(self):
        return self.nombre

    class Meta:
        db_table = 'jornada'
        verbose_name = "Jornada"
        verbose_name_plural = "Jornadas"


class Grado(models.Model):
    nombre = models.CharField(max_length=20, unique=True)

    def __str__(self):
        return self.nombre

    class Meta:
        db_table = 'grado'
        verbose_name = "Grado"
        verbose_name_plural = "Grados"


class Grupo(models.Model):
    nombre = models.CharField(max_length=10)
    grado = models.ForeignKey(Grado, on_delete=models.CASCADE, related_name='grupos')
    sede = models.ForeignKey(Sede, on_delete=models.CASCADE, related_name='grupos')
    jornada = models.ForeignKey(Jornada, on_delete=models.CASCADE, related_name='grupos')

    def __str__(self):
        return f"{self.grado.nombre} {self.nombre} - {self.sede.nombre} ({self.jornada.nombre})"

    class Meta:
        db_table = 'grupo'
        verbose_name = "Grupo"
        verbose_name_plural = "Grupos"
        unique_together = ['nombre', 'grado', 'sede', 'jornada']


class Estudiante(models.Model):
    nombres = models.CharField(max_length=100)
    apellidos = models.CharField(max_length=100)
    documento_identidad = models.CharField(max_length=20)
    tipo_documento = models.CharField(max_length=2, choices=TIPO_DOCUMENTO_CHOICES)
    fecha_nacimiento = models.DateField(null=True, blank=True)
    telefono = models.CharField(max_length=20, blank=True, null=True)
    correo_electronico = models.EmailField(blank=True, null=True)
    direccion = models.CharField(max_length=150, blank=True, null=True)
    municipio = models.ForeignKey(Municipio, on_delete=models.SET_NULL, null=True, blank=True, related_name='estudiantes')
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='estudiantes_creados')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='estudiantes_modificados')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"{self.apellidos}, {self.nombres} ({self.documento_identidad})"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'estudiante'
        verbose_name = "Estudiante"
        verbose_name_plural = "Estudiantes"
        unique_together = ['documento_identidad', 'tipo_documento']


class Docente(models.Model):
    nombres = models.CharField(max_length=100)
    apellidos = models.CharField(max_length=100)
    documento_identidad = models.CharField(max_length=20)
    tipo_documento = models.CharField(max_length=2, choices=TIPO_DOCUMENTO_CHOICES)
    profesion = models.CharField(max_length=100, blank=True, null=True)
    telefono = models.CharField(max_length=20, blank=True, null=True)
    correo_electronico = models.EmailField(blank=True, null=True)
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='docentes_creados')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='docentes_modificados')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"{self.apellidos}, {self.nombres} ({self.documento_identidad})"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'docente'
        verbose_name = "Docente"
        verbose_name_plural = "Docentes"
        unique_together = ['documento_identidad', 'tipo_documento']


class Acudiente(models.Model):
    nombres = models.CharField(max_length=100)
    apellidos = models.CharField(max_length=100)
    documento_identidad = models.CharField(max_length=20)
    tipo_documento = models.CharField(max_length=2, choices=TIPO_DOCUMENTO_CHOICES)
    telefono = models.CharField(max_length=20, blank=True, null=True)
    direccion = models.CharField(max_length=150, blank=True, null=True)
    correo_electronico = models.EmailField(blank=True, null=True)
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='acudientes_creados')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='acudientes_modificados')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"{self.apellidos}, {self.nombres} ({self.documento_identidad})"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'acudiente'
        verbose_name = "Acudiente"
        verbose_name_plural = "Acudientes"
        unique_together = ['documento_identidad', 'tipo_documento']


class EstudianteAcudiente(models.Model):
    estudiante = models.ForeignKey(Estudiante, on_delete=models.CASCADE, related_name='acudientes')
    acudiente = models.ForeignKey(Acudiente, on_delete=models.CASCADE, related_name='estudiantes')
    parentesco = models.CharField(max_length=50, blank=True, null=True)
    es_principal = models.BooleanField(default=False)

    def __str__(self):
        return f"{self.acudiente} - {self.estudiante} ({self.parentesco})"

    class Meta:
        db_table = 'estudiante_acudiente'
        verbose_name = "Relación Estudiante-Acudiente"
        verbose_name_plural = "Relaciones Estudiante-Acudiente"
        unique_together = ['estudiante', 'acudiente']


class Asignatura(models.Model):
    nombre = models.CharField(max_length=100, unique=True)
    intensidad_horaria = models.PositiveIntegerField(null=True, blank=True)

    def __str__(self):
        return self.nombre

    class Meta:
        db_table = 'asignatura'
        verbose_name = "Asignatura"
        verbose_name_plural = "Asignaturas"


class GradoAsignatura(models.Model):
    grado = models.ForeignKey(Grado, on_delete=models.CASCADE, related_name='asignaturas')
    asignatura = models.ForeignKey(Asignatura, on_delete=models.CASCADE, related_name='grados')

    def __str__(self):
        return f"{self.asignatura.nombre} - {self.grado.nombre}"

    class Meta:
        db_table = 'grado_asignatura'
        verbose_name = "Asignatura por Grado"
        verbose_name_plural = "Asignaturas por Grado"
        unique_together = ['grado', 'asignatura']


class AnioLectivo(models.Model):
    anio = models.PositiveIntegerField(unique=True)
    fecha_inicio = models.DateField()
    fecha_fin = models.DateField()
    estado = models.CharField(max_length=20, choices=ESTADO_ANIO_LECTIVO_CHOICES, default='ACTIVO')

    def clean(self):
        from django.core.exceptions import ValidationError
        if self.fecha_inicio >= self.fecha_fin:
            raise ValidationError('La fecha de inicio debe ser anterior a la fecha de fin.')

    def __str__(self):
        return f"{self.anio} ({self.get_estado_display()})"

    class Meta:
        db_table = 'anio_lectivo'
        verbose_name = "Año Lectivo"
        verbose_name_plural = "Años Lectivos"


class Periodo(models.Model):
    nombre = models.CharField(max_length=50)
    fecha_inicio = models.DateField()
    fecha_fin = models.DateField()
    anio_lectivo = models.ForeignKey(AnioLectivo, on_delete=models.CASCADE, related_name='periodos')

    def clean(self):
        from django.core.exceptions import ValidationError
        if self.fecha_inicio >= self.fecha_fin:
            raise ValidationError('La fecha de inicio debe ser anterior a la fecha de fin.')

    def __str__(self):
        return f"{self.nombre} - {self.anio_lectivo.anio}"

    class Meta:
        db_table = 'periodo'
        verbose_name = "Periodo Académico"
        verbose_name_plural = "Periodos Académicos"


class CargaAcademica(models.Model):
    docente = models.ForeignKey(Docente, on_delete=models.CASCADE, related_name='cargas_academicas')
    grupo = models.ForeignKey(Grupo, on_delete=models.CASCADE, related_name='cargas_academicas')
    asignatura = models.ForeignKey(Asignatura, on_delete=models.CASCADE, related_name='cargas_academicas')
    anio_lectivo = models.ForeignKey(AnioLectivo, on_delete=models.CASCADE, related_name='cargas_academicas')
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='cargas_academicas_creadas')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='cargas_academicas_modificadas')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"{self.asignatura.nombre} - {self.grupo} - {self.docente} ({self.anio_lectivo.anio})"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'carga_academica'
        verbose_name = "Carga Académica"
        verbose_name_plural = "Cargas Académicas"
        unique_together = ['docente', 'grupo', 'asignatura', 'anio_lectivo']


class Matricula(models.Model):
    estudiante = models.ForeignKey(Estudiante, on_delete=models.CASCADE, related_name='matriculas')
    grupo = models.ForeignKey(Grupo, on_delete=models.CASCADE, related_name='matriculas')
    anio_lectivo = models.ForeignKey(AnioLectivo, on_delete=models.CASCADE, related_name='matriculas')
    fecha_matricula = models.DateField(default=timezone.now)
    estado = models.CharField(max_length=20, choices=ESTADO_MATRICULA_CHOICES, default='ACTIVO')
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='matriculas_creadas')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='matriculas_modificadas')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"{self.estudiante} - {self.grupo} ({self.anio_lectivo.anio})"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'matricula'
        verbose_name = "Matrícula"
        verbose_name_plural = "Matrículas"
        unique_together = ['estudiante', 'grupo', 'anio_lectivo']


class TipoEvaluacion(models.Model):
    nombre = models.CharField(max_length=50, unique=True)
    descripcion = models.CharField(max_length=255, blank=True, null=True)

    def __str__(self):
        return self.nombre

    class Meta:
        db_table = 'tipo_evaluacion'
        verbose_name = "Tipo de Evaluación"
        verbose_name_plural = "Tipos de Evaluación"


class Competencia(models.Model):
    nombre = models.CharField(max_length=200)
    descripcion = models.TextField(blank=True, null=True)
    asignatura = models.ForeignKey(Asignatura, on_delete=models.CASCADE, related_name='competencias')
    grado = models.ForeignKey(Grado, on_delete=models.CASCADE, related_name='competencias')

    def __str__(self):
        return f"{self.nombre} - {self.asignatura.nombre} ({self.grado.nombre})"

    class Meta:
        db_table = 'competencia'
        verbose_name = "Competencia"
        verbose_name_plural = "Competencias"


class Logro(models.Model):
    descripcion = models.TextField()
    competencia = models.ForeignKey(Competencia, on_delete=models.CASCADE, related_name='logros')
    periodo = models.ForeignKey(Periodo, on_delete=models.CASCADE, related_name='logros')
    porcentaje = models.DecimalField(max_digits=5, decimal_places=2, validators=[MinValueValidator(0), MaxValueValidator(100)], blank=True, null=True)
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='logros_creados')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='logros_modificados')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"{self.descripcion[:50]}... - {self.competencia.nombre} ({self.periodo.nombre})"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'logro'
        verbose_name = "Logro"
        verbose_name_plural = "Logros"


class Evaluacion(models.Model):
    carga_academica = models.ForeignKey(CargaAcademica, on_delete=models.CASCADE, related_name='evaluaciones')
    periodo = models.ForeignKey(Periodo, on_delete=models.CASCADE, related_name='evaluaciones')
    tipo_evaluacion = models.ForeignKey(TipoEvaluacion, on_delete=models.CASCADE, related_name='evaluaciones')
    logro = models.ForeignKey(Logro, on_delete=models.CASCADE, related_name='evaluaciones', null=True, blank=True)
    nombre = models.CharField(max_length=100)
    descripcion = models.TextField(blank=True, null=True)
    fecha_presentacion = models.DateField(blank=True, null=True)
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='evaluaciones_creadas')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='evaluaciones_modificadas')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"{self.nombre} - {self.carga_academica.asignatura.nombre} ({self.periodo.nombre})"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'evaluacion'
        verbose_name = "Evaluación"
        verbose_name_plural = "Evaluaciones"


class Calificacion(models.Model):
    evaluacion = models.ForeignKey(Evaluacion, on_delete=models.CASCADE, related_name='calificaciones')
    matricula = models.ForeignKey(Matricula, on_delete=models.CASCADE, related_name='calificaciones')
    nota = models.DecimalField(max_digits=4, decimal_places=2, validators=[MinValueValidator(0), MaxValueValidator(5)])
    observaciones = models.CharField(max_length=255, blank=True, null=True)
    fecha_calificacion = models.DateTimeField(default=timezone.now)
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='calificaciones_creadas')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='calificaciones_modificadas')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"{self.evaluacion.nombre} - {self.matricula.estudiante} - Nota: {self.nota}"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'calificacion'
        verbose_name = "Calificación"
        verbose_name_plural = "Calificaciones"
        unique_together = ['evaluacion', 'matricula']


class ActividadRecuperacion(models.Model):
    calificacion = models.ForeignKey(Calificacion, on_delete=models.CASCADE, related_name='recuperaciones')
    fecha = models.DateField()
    descripcion = models.TextField(blank=True, null=True)
    nota_recuperacion = models.DecimalField(max_digits=4, decimal_places=2, validators=[MinValueValidator(0), MaxValueValidator(5)])
    fecha_calificacion = models.DateTimeField(blank=True, null=True)
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='recuperaciones_creadas')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='recuperaciones_modificadas')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"Recuperación de {self.calificacion} - Nota: {self.nota_recuperacion}"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'actividad_recuperacion'
        verbose_name = "Actividad de Recuperación"
        verbose_name_plural = "Actividades de Recuperación"


class Pregunta(models.Model):
    evaluacion = models.ForeignKey(Evaluacion, on_delete=models.CASCADE, related_name='preguntas')
    enunciado = models.CharField(max_length=500)
    tipo = models.CharField(max_length=20, choices=TIPO_PREGUNTA_CHOICES)
    peso = models.DecimalField(max_digits=3, decimal_places=2, default=1.0, validators=[MinValueValidator(0.01)])

    def __str__(self):
        return f"{self.enunciado[:50]}... - {self.evaluacion.nombre}"

    class Meta:
        db_table = 'pregunta'
        verbose_name = "Pregunta"
        verbose_name_plural = "Preguntas"


class OpcionRespuesta(models.Model):
    pregunta = models.ForeignKey(Pregunta, on_delete=models.CASCADE, related_name='opciones')
    texto = models.CharField(max_length=300)
    es_correcta = models.BooleanField(default=False)

    def __str__(self):
        return f"{self.texto[:50]}... {'(Correcta)' if self.es_correcta else ''}"

    class Meta:
        db_table = 'opcion_respuesta'
        verbose_name = "Opción de Respuesta"
        verbose_name_plural = "Opciones de Respuesta"


class RespuestaEstudiante(models.Model):
    pregunta = models.ForeignKey(Pregunta, on_delete=models.CASCADE, related_name='respuestas')
    calificacion = models.ForeignKey(Calificacion, on_delete=models.CASCADE, related_name='respuestas')
    respuesta = models.CharField(max_length=1000, blank=True, null=True)
    puntaje_obtenido = models.DecimalField(max_digits=4, decimal_places=2, blank=True, null=True)
    opcion_seleccionada = models.ForeignKey(OpcionRespuesta, on_delete=models.SET_NULL, related_name='selecciones', blank=True, null=True)

    def __str__(self):
        return f"Respuesta a {self.pregunta} por {self.calificacion.matricula.estudiante}"

    class Meta:
        db_table = 'respuesta_estudiante'
        verbose_name = "Respuesta de Estudiante"
        verbose_name_plural = "Respuestas de Estudiantes"


class Asistencia(models.Model):
    matricula = models.ForeignKey(Matricula, on_delete=models.CASCADE, related_name='asistencias')
    fecha = models.DateField()
    estado = models.CharField(max_length=20, choices=ESTADO_ASISTENCIA_CHOICES)
    observaciones = models.CharField(max_length=255, blank=True, null=True)
    fecha_registro = models.DateTimeField(default=timezone.now)
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='asistencias_creadas')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='asistencias_modificadas')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"{self.matricula.estudiante} - {self.fecha} - {self.get_estado_display()}"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'asistencia'
        verbose_name = "Asistencia"
        verbose_name_plural = "Asistencias"
        unique_together = ['matricula', 'fecha']


class ObservacionComportamiento(models.Model):
    matricula = models.ForeignKey(Matricula, on_delete=models.CASCADE, related_name='observaciones')
    docente_observador = models.ForeignKey(Docente, on_delete=models.CASCADE, related_name='observaciones_realizadas')
    carga_academica = models.ForeignKey(CargaAcademica, on_delete=models.SET_NULL, related_name='observaciones', blank=True, null=True)
    fecha_hora = models.DateTimeField(default=timezone.now)
    descripcion = models.TextField()
    tipo_observacion = models.CharField(max_length=50, blank=True, null=True)
    acciones_tomadas = models.CharField(max_length=500, blank=True, null=True)
    
    # Campos de auditoría
    usuario_creacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='observaciones_creadas')
    fecha_creacion = models.DateTimeField(default=timezone.now)
    usuario_modificacion = models.ForeignKey('Usuario', on_delete=models.SET_NULL, null=True, related_name='observaciones_modificadas')
    fecha_modificacion = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"{self.matricula.estudiante} - {self.fecha_hora} - {self.tipo_observacion}"

    def save(self, *args, **kwargs):
        if self.pk:
            self.fecha_modificacion = timezone.now()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'observacion_comportamiento'
        verbose_name = "Observación de Comportamiento"
        verbose_name_plural = "Observaciones de Comportamiento"


class DiaSemana(models.Model):
    nombre = models.CharField(max_length=20, unique=True)
    codigo = models.SmallIntegerField(unique=True)  # 1=Lunes, 2=Martes, etc.

    def __str__(self):
        return self.nombre

    class Meta:
        db_table = 'dia_semana'
        verbose_name = "Día de la Semana"
        verbose_name_plural = "Días de la Semana"


class Horario(models.Model):
    carga_academica = models.ForeignKey(CargaAcademica, on_delete=models.CASCADE, related_name='horarios')
    dia = models.ForeignKey(DiaSemana, on_delete=models.CASCADE, related_name='horarios')
    hora_inicio = models.TimeField()
    hora_fin = models.TimeField()

    def clean(self):
        from django.core.exceptions import ValidationError
        if self.hora_inicio >= self.hora_fin:
            raise ValidationError('La hora de inicio debe ser anterior a la hora de fin.')

    def __str__(self):
        return f"{self.carga_academica.asignatura.nombre} - {self.dia.nombre} ({self.hora_inicio} - {self.hora_fin})"

    class Meta:
        db_table = 'horario'
        verbose_name = "Horario"
        verbose_name_plural = "Horarios"
        unique_together = ['carga_academica', 'dia', 'hora_inicio']


class Modulo(models.Model):
    nombre = models.CharField(max_length=50, unique=True)
    descripcion = models.CharField(max_length=255, blank=True, null=True)
    codigo = models.CharField(max_length=30, unique=True)

    def __str__(self):
        return self.nombre

    class Meta:
        db_table = 'modulo'
        verbose_name = "Módulo del Sistema"
        verbose_name_plural = "Módulos del Sistema"
        ordering = ['nombre']
        permissions = [
            ("can_access", "Puede acceder al módulo"),
            ("can_create", "Puede crear registros"),
            ("can_read", "Puede leer registros"),
            ("can_update", "Puede actualizar registros"),
            ("can_delete", "Puede eliminar registros"),
        ]
class Permiso(models.Model):
    nombre = models.CharField(max_length=50, unique=True)
    descripcion = models.CharField(max_length=255, blank=True, null=True)
    modulo = models.ForeignKey(Modulo, on_delete=models.CASCADE, related_name='permisos')

    def __str__(self):
        return f"{self.nombre} - {self.modulo.nombre}"

    class Meta:
        db_table = 'permiso'
        verbose_name = "Permiso"
        verbose_name_plural = "Permisos"
        ordering = ['modulo', 'nombre']         
        unique_together = ['nombre', 'modulo']
        permissions = [
            ("can_access", "Puede acceder al permiso"),
            ("can_create", "Puede crear registros"),
            ("can_read", "Puede leer registros"),
            ("can_update", "Puede actualizar registros"),
            ("can_delete", "Puede eliminar registros"),
        ]   

class Rol(models.Model):
    nombre = models.CharField(max_length=50, unique=True)
    descripcion = models.CharField(max_length=255, blank=True, null=True)
    permisos = models.ManyToManyField(Permiso, related_name='roles')

    def __str__(self):
        return self.nombre

    class Meta:
        db_table = 'rol'
        verbose_name = "Rol"
        verbose_name_plural = "Roles"
        ordering = ['nombre']
        permissions = [
            ("can_access", "Puede acceder al rol"),
            ("can_create", "Puede crear registros"),
            ("can_read", "Puede leer registros"),
            ("can_update", "Puede actualizar registros"),
            ("can_delete", "Puede eliminar registros"),
        ]
class Usuario(models.Model):
    username = models.CharField(max_length=30, unique=True)
    password = models.CharField(max_length=128)
    email = models.EmailField(unique=True)
    rol = models.ForeignKey(Rol, on_delete=models.CASCADE, related_name='usuarios')
    fecha_creacion = models.DateTimeField(auto_now_add=True)
    fecha_modificacion = models.DateTimeField(auto_now=True)

    def __str__(self):
        return self.username

    def save(self, *args, **kwargs):
        if not self.pk or 'password' in self.changed_data:
            self.password = hashlib.sha256(self.password.encode()).hexdigest()
        super().save(*args, **kwargs)

    class Meta:
        db_table = 'usuario'
        verbose_name = "Usuario"
        verbose_name_plural = "Usuarios"
        ordering = ['username']
        permissions = [
            ("can_access", "Puede acceder al usuario"),
            ("can_create", "Puede crear registros"),
            ("can_read", "Puede leer registros"),
            ("can_update", "Puede actualizar registros"),
            ("can_delete", "Puede eliminar registros"),
        ]
class Archivo(models.Model):
    nombre = models.CharField(max_length=100)
    archivo = models.FileField(upload_to='archivos/')
    fecha_subida = models.DateTimeField(auto_now_add=True)
    usuario = models.ForeignKey(Usuario, on_delete=models.CASCADE, related_name='archivos_subidos')

    def __str__(self):
        return self.nombre

    class Meta:
        db_table = 'archivo'
        verbose_name = "Archivo"
        verbose_name_plural = "Archivos"
        ordering = ['fecha_subida']
        permissions = [
            ("can_access", "Puede acceder al archivo"),
            ("can_create", "Puede crear registros"),
            ("can_read", "Puede leer registros"),
            ("can_update", "Puede actualizar registros"),
            ("can_delete", "Puede eliminar registros"),
        ]
    def save(self, *args, **kwargs):
        # Generar un nombre único para el archivo
        if not self.pk:
            self.nombre = f"{self.usuario.username}_{self.nombre}"
        super().save(*args, **kwargs)
        # Renombrar el archivo en el sistema de archivos
        if self.pk:
            nuevo_nombre = f"{self.usuario.username}_{self.nombre}"
            nuevo_ruta = os.path.join(os.path.dirname(self.archivo.path), nuevo_nombre)
            os.rename(self.archivo.path, nuevo_ruta)
            self.archivo.name = nuevo_nombre
            super().save(update_fields=['archivo'])
        # Eliminar el archivo del sistema de archivos al eliminar el objeto
        if self.pk:
            os.remove(self.archivo.path)
        super().delete(*args, **kwargs)
    def delete(self, *args, **kwargs):
        # Eliminar el archivo del sistema de archivos
        if self.archivo:
            if os.path.isfile(self.archivo.path):
                os.remove(self.archivo.path)
        super().delete(*args, **kwargs)
    def get_absolute_url(self):
        return os.path.join(settings.MEDIA_URL, self.archivo.name)
    def get_file_size(self):
        return self.archivo.size
    def get_file_extension(self):
        return os.path.splitext(self.archivo.name)[1]
    def get_file_name(self):
        return os.path.splitext(self.archivo.name)[0]
    def get_file_path(self):    
        return self.archivo.path    
    def get_file_url(self):
        return os.path.join(settings.MEDIA_URL, self.archivo.name)
    def get_file_type(self):
        return self.archivo.file.content_type
    def get_file_mime_type(self):
        return self.archivo.file.content_type
    def get_file_size_formatted(self):
        size = self.archivo.size
        if size < 1024:
            return f"{size} bytes"
        elif size < 1048576:
            return f"{size / 1024:.2f} KB"
        elif size < 1073741824:
            return f"{size / 1048576:.2f} MB"
        else:
            return f"{size / 1073741824:.2f} GB"
    def get_file_creation_date(self):
        return self.fecha_subida.strftime("%Y-%m-%d %H:%M:%S")
    def get_file_modification_date(self):
        return self.fecha_modificacion.strftime("%Y-%m-%d %H:%M:%S")
    def get_file_upload_date(self):
        return self.fecha_subida.strftime("%Y-%m-%d %H:%M:%S")
    def get_file_upload_time(self):
        return self.fecha_subida.strftime("%H:%M:%S")   
    def get_file_upload_date_time(self):
        return self.fecha_subida.strftime("%Y-%m-%d %H:%M:%S")
    def get_file_upload_date_time_formatted(self):
        return self.fecha_subida.strftime("%d/%m/%Y %H:%M:%S")                  
    def get_file_upload_date_formatted(self):
        return self.fecha_subida.strftime("%d/%m/%Y")   
    def get_file_upload_time_formatted(self):           
        return self.fecha_subida.strftime("%H:%M:%S")
    def get_file_upload_date_time_formatted(self):
        return self.fecha_subida.strftime("%d/%m/%Y %H:%M:%S")
    def get_file_upload_date_time_formatted(self):
        return self.fecha_subida.strftime("%d/%m/%Y %H:%M:%S")  
    def get_file_upload_date_time_formatted(self):
        return self.fecha_subida.strftime("%d/%m/%Y %H:%M:%S")
    