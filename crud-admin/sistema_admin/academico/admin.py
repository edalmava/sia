from django.contrib import admin
from .models import (
    Departamento, Municipio, Institucion, Sede, Jornada, Grado, Grupo,
    Estudiante, Docente, Acudiente, EstudianteAcudiente, Asignatura,
    GradoAsignatura, AnioLectivo, Periodo, CargaAcademica, Matricula,
    TipoEvaluacion, Competencia, Logro, Evaluacion, Calificacion,
    ActividadRecuperacion, Pregunta, OpcionRespuesta, RespuestaEstudiante,
    Asistencia, ObservacionComportamiento, DiaSemana, Horario,
    Modulo, Permiso, Rol, Usuario, Archivo
)

# Registros básicos con clases personalizadas para mejorar la visualización

@admin.register(Departamento)
class DepartamentoAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'codigo')
    search_fields = ('nombre', 'codigo')
    ordering = ('nombre',)

@admin.register(Municipio)
class MunicipioAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'departamento', 'codigo')
    search_fields = ('nombre', 'codigo')
    list_filter = ('departamento',)
    ordering = ('departamento', 'nombre')

@admin.register(Institucion)
class InstitucionAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'codigo_dane')
    search_fields = ('nombre', 'codigo_dane')
    ordering = ('nombre',)

@admin.register(Sede)
class SedeAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'institucion', 'municipio', 'direccion')
    search_fields = ('nombre', 'direccion')
    list_filter = ('institucion', 'municipio')
    ordering = ('institucion', 'nombre')

@admin.register(Jornada)
class JornadaAdmin(admin.ModelAdmin):
    list_display = ('nombre',)
    search_fields = ('nombre',)
    ordering = ('nombre',)

@admin.register(Grado)
class GradoAdmin(admin.ModelAdmin):
    list_display = ('nombre',)
    search_fields = ('nombre',)
    ordering = ('nombre',)

@admin.register(Grupo)
class GrupoAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'grado', 'sede', 'jornada')
    search_fields = ('nombre',)
    list_filter = ('grado', 'sede', 'jornada')
    ordering = ('grado', 'nombre')

# Personas

@admin.register(Estudiante)
class EstudianteAdmin(admin.ModelAdmin):
    list_display = ('apellidos', 'nombres', 'documento_identidad', 'tipo_documento', 'fecha_nacimiento', 'telefono', 'correo_electronico')
    search_fields = ('nombres', 'apellidos', 'documento_identidad')
    list_filter = ('tipo_documento', 'municipio')
    date_hierarchy = 'fecha_creacion'
    fieldsets = (
        ('Información Personal', {
            'fields': ('nombres', 'apellidos', 'documento_identidad', 'tipo_documento', 'fecha_nacimiento')
        }),
        ('Información de Contacto', {
            'fields': ('telefono', 'correo_electronico', 'direccion', 'municipio')
        }),
        ('Auditoría', {
            'fields': ('usuario_creacion', 'fecha_creacion', 'usuario_modificacion', 'fecha_modificacion'),
            'classes': ('collapse',)
        })
    )
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')

@admin.register(Docente)
class DocenteAdmin(admin.ModelAdmin):
    list_display = ('apellidos', 'nombres', 'documento_identidad', 'tipo_documento', 'profesion', 'telefono', 'correo_electronico')
    search_fields = ('nombres', 'apellidos', 'documento_identidad')
    list_filter = ('tipo_documento', 'profesion')
    date_hierarchy = 'fecha_creacion'
    fieldsets = (
        ('Información Personal', {
            'fields': ('nombres', 'apellidos', 'documento_identidad', 'tipo_documento', 'profesion')
        }),
        ('Información de Contacto', {
            'fields': ('telefono', 'correo_electronico')
        }),
        ('Auditoría', {
            'fields': ('usuario_creacion', 'fecha_creacion', 'usuario_modificacion', 'fecha_modificacion'),
            'classes': ('collapse',)
        })
    )
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')

@admin.register(Acudiente)
class AcudienteAdmin(admin.ModelAdmin):
    list_display = ('apellidos', 'nombres', 'documento_identidad', 'tipo_documento', 'telefono', 'correo_electronico')
    search_fields = ('nombres', 'apellidos', 'documento_identidad')
    list_filter = ('tipo_documento',)
    date_hierarchy = 'fecha_creacion'
    fieldsets = (
        ('Información Personal', {
            'fields': ('nombres', 'apellidos', 'documento_identidad', 'tipo_documento')
        }),
        ('Información de Contacto', {
            'fields': ('telefono', 'correo_electronico', 'direccion')
        }),
        ('Auditoría', {
            'fields': ('usuario_creacion', 'fecha_creacion', 'usuario_modificacion', 'fecha_modificacion'),
            'classes': ('collapse',)
        })
    )
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')

@admin.register(EstudianteAcudiente)
class EstudianteAcudienteAdmin(admin.ModelAdmin):
    list_display = ('estudiante', 'acudiente', 'parentesco', 'es_principal')
    search_fields = ('estudiante__nombres', 'estudiante__apellidos', 'acudiente__nombres', 'acudiente__apellidos', 'parentesco')
    list_filter = ('es_principal', 'parentesco')
    autocomplete_fields = ['estudiante', 'acudiente']

# Académico

@admin.register(Asignatura)
class AsignaturaAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'intensidad_horaria')
    search_fields = ('nombre',)
    ordering = ('nombre',)

@admin.register(GradoAsignatura)
class GradoAsignaturaAdmin(admin.ModelAdmin):
    list_display = ('grado', 'asignatura')
    list_filter = ('grado', 'asignatura')
    autocomplete_fields = ['grado', 'asignatura']

@admin.register(AnioLectivo)
class AnioLectivoAdmin(admin.ModelAdmin):
    list_display = ('anio', 'fecha_inicio', 'fecha_fin', 'estado')
    search_fields = ('anio',)
    list_filter = ('estado',)
    ordering = ('-anio',)

@admin.register(Periodo)
class PeriodoAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'anio_lectivo', 'fecha_inicio', 'fecha_fin')
    search_fields = ('nombre',)
    list_filter = ('anio_lectivo',)
    ordering = ('anio_lectivo', 'fecha_inicio')

@admin.register(CargaAcademica)
class CargaAcademicaAdmin(admin.ModelAdmin):
    list_display = ('docente', 'grupo', 'asignatura', 'anio_lectivo')
    search_fields = ('docente__nombres', 'docente__apellidos', 'grupo__nombre', 'asignatura__nombre')
    list_filter = ('anio_lectivo', 'grupo__grado', 'grupo__sede')
    autocomplete_fields = ['docente', 'grupo', 'asignatura', 'anio_lectivo']
    date_hierarchy = 'fecha_creacion'
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')

@admin.register(Matricula)
class MatriculaAdmin(admin.ModelAdmin):
    list_display = ('estudiante', 'grupo', 'anio_lectivo', 'fecha_matricula', 'estado')
    search_fields = ('estudiante__nombres', 'estudiante__apellidos', 'estudiante__documento_identidad')
    list_filter = ('anio_lectivo', 'grupo__grado', 'grupo__sede', 'estado')
    date_hierarchy = 'fecha_matricula'
    autocomplete_fields = ['estudiante', 'grupo', 'anio_lectivo']
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')

# Evaluaciones

@admin.register(TipoEvaluacion)
class TipoEvaluacionAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'descripcion')
    search_fields = ('nombre',)
    ordering = ('nombre',)

@admin.register(Competencia)
class CompetenciaAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'asignatura', 'grado')
    search_fields = ('nombre', 'descripcion')
    list_filter = ('asignatura', 'grado')
    autocomplete_fields = ['asignatura', 'grado']

@admin.register(Logro)
class LogroAdmin(admin.ModelAdmin):
    list_display = ('descripcion_corta', 'competencia', 'periodo', 'porcentaje')
    search_fields = ('descripcion',)
    list_filter = ('competencia__asignatura', 'periodo')
    autocomplete_fields = ['competencia', 'periodo']
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')
    
    def descripcion_corta(self, obj):
        return obj.descripcion[:100] + '...' if len(obj.descripcion) > 100 else obj.descripcion
    descripcion_corta.short_description = 'Descripción'

@admin.register(Evaluacion)
class EvaluacionAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'carga_academica', 'periodo', 'tipo_evaluacion', 'logro', 'fecha_presentacion')
    search_fields = ('nombre', 'descripcion')
    list_filter = ('tipo_evaluacion', 'periodo', 'carga_academica__asignatura')
    autocomplete_fields = ['carga_academica', 'periodo', 'tipo_evaluacion', 'logro']
    date_hierarchy = 'fecha_presentacion'
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')

@admin.register(Calificacion)
class CalificacionAdmin(admin.ModelAdmin):
    list_display = ('evaluacion', 'matricula', 'nota', 'fecha_calificacion')
    search_fields = ('matricula__estudiante__nombres', 'matricula__estudiante__apellidos', 'evaluacion__nombre')
    list_filter = ('evaluacion__carga_academica__asignatura', 'evaluacion__periodo')
    autocomplete_fields = ['evaluacion', 'matricula']
    date_hierarchy = 'fecha_calificacion'
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')

@admin.register(ActividadRecuperacion)
class ActividadRecuperacionAdmin(admin.ModelAdmin):
    list_display = ('calificacion', 'fecha', 'nota_recuperacion', 'fecha_calificacion')
    search_fields = ('calificacion__matricula__estudiante__nombres', 'calificacion__matricula__estudiante__apellidos')
    list_filter = ('calificacion__evaluacion__carga_academica__asignatura', 'fecha')
    autocomplete_fields = ['calificacion']
    date_hierarchy = 'fecha'
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')

@admin.register(Pregunta)
class PreguntaAdmin(admin.ModelAdmin):
    list_display = ('enunciado_corto', 'evaluacion', 'tipo', 'peso')
    search_fields = ('enunciado',)
    list_filter = ('evaluacion', 'tipo')
    autocomplete_fields = ['evaluacion']
    
    def enunciado_corto(self, obj):
        return obj.enunciado[:100] + '...' if len(obj.enunciado) > 100 else obj.enunciado
    enunciado_corto.short_description = 'Enunciado'

@admin.register(OpcionRespuesta)
class OpcionRespuestaAdmin(admin.ModelAdmin):
    list_display = ('texto_corto', 'pregunta', 'es_correcta')
    search_fields = ('texto',)
    list_filter = ('pregunta__evaluacion', 'es_correcta')
    autocomplete_fields = ['pregunta']
    
    def texto_corto(self, obj):
        return obj.texto[:100] + '...' if len(obj.texto) > 100 else obj.texto
    texto_corto.short_description = 'Texto'

@admin.register(RespuestaEstudiante)
class RespuestaEstudianteAdmin(admin.ModelAdmin):
    list_display = ('pregunta', 'calificacion', 'opcion_seleccionada', 'puntaje_obtenido')
    search_fields = ('respuesta', 'calificacion__matricula__estudiante__nombres', 'calificacion__matricula__estudiante__apellidos')
    list_filter = ('pregunta__evaluacion', 'pregunta__tipo')
    autocomplete_fields = ['pregunta', 'calificacion', 'opcion_seleccionada']

# Asistencia

@admin.register(Asistencia)
class AsistenciaAdmin(admin.ModelAdmin):
    list_display = ('matricula', 'fecha', 'estado', 'observaciones')
    search_fields = ('matricula__estudiante__nombres', 'matricula__estudiante__apellidos', 'observaciones')
    list_filter = ('estado', 'fecha', 'matricula__grupo')
    date_hierarchy = 'fecha'
    autocomplete_fields = ['matricula']
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')

@admin.register(ObservacionComportamiento)
class ObservacionComportamientoAdmin(admin.ModelAdmin):
    list_display = ('matricula', 'docente_observador', 'fecha_hora', 'tipo_observacion')
    search_fields = ('matricula__estudiante__nombres', 'matricula__estudiante__apellidos', 'descripcion')
    list_filter = ('tipo_observacion', 'docente_observador', 'fecha_hora')
    date_hierarchy = 'fecha_hora'
    autocomplete_fields = ['matricula', 'docente_observador', 'carga_academica']
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')

# Horarios

@admin.register(DiaSemana)
class DiaSemanaAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'codigo')
    search_fields = ('nombre',)
    ordering = ('codigo',)

@admin.register(Horario)
class HorarioAdmin(admin.ModelAdmin):
    list_display = ('carga_academica', 'dia', 'hora_inicio', 'hora_fin')
    search_fields = ('carga_academica__docente__nombres', 'carga_academica__docente__apellidos', 'carga_academica__asignatura__nombre')
    list_filter = ('dia', 'carga_academica__grupo', 'carga_academica__asignatura')
    autocomplete_fields = ['carga_academica', 'dia']
    ordering = ('dia__codigo', 'hora_inicio')

# Sistema

@admin.register(Modulo)
class ModuloAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'codigo', 'descripcion')
    search_fields = ('nombre', 'codigo')
    ordering = ('nombre',)

@admin.register(Permiso)
class PermisoAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'modulo', 'descripcion')
    search_fields = ('nombre', 'descripcion')
    list_filter = ('modulo',)
    autocomplete_fields = ['modulo']

@admin.register(Rol)
class RolAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'descripcion')
    search_fields = ('nombre', 'descripcion')
    filter_horizontal = ('permisos',)

@admin.register(Usuario)
class UsuarioAdmin(admin.ModelAdmin):
    list_display = ('username', 'email', 'rol', 'fecha_creacion')
    search_fields = ('username', 'email')
    list_filter = ('rol',)
    date_hierarchy = 'fecha_creacion'
    autocomplete_fields = ['rol']
    readonly_fields = ('fecha_creacion', 'fecha_modificacion')
    fieldsets = (
        ('Información de Usuario', {
            'fields': ('username', 'password', 'email', 'rol')
        }),
        ('Fechas', {
            'fields': ('fecha_creacion', 'fecha_modificacion'),
            'classes': ('collapse',)
        })
    )

@admin.register(Archivo)
class ArchivoAdmin(admin.ModelAdmin):
    list_display = ('nombre', 'usuario', 'fecha_subida', 'get_file_size_formatted')
    search_fields = ('nombre', 'usuario__username')
    list_filter = ('usuario', 'fecha_subida')
    date_hierarchy = 'fecha_subida'
    autocomplete_fields = ['usuario']
    readonly_fields = ('fecha_subida',)