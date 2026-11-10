# BIODENTIS Escritorio

Demo funcional para una clínica odontológica de una sede y un equipo. Incluye acceso con usuario y contraseña, agenda, pacientes, historia clínica, odontograma, procedimientos, consentimiento de prueba, radiografías, cobros y auditoría. El ejecutable sirve las pantallas y guarda los datos en SQLite en la misma PC.

## Abrir la demo

1. En Windows de 64 bits con Microsoft Edge WebView2 Runtime, abre `ABRIR-ESCRITORIO.cmd` o `bin/BIODENTIS-Escritorio.exe`.
2. Ingresa con el usuario `biodentis.demo` y la contraseña temporal `BiodentisDemo2026!`.
3. Trabaja con datos ficticios. La demo crea su base en `%LOCALAPPDATA%\BIODENTIS\EscritorioDemo\datos` y conserva los cambios entre sesiones. Si cambias la contraseña, seguirá vigente al volver a abrir la demo.

La demo compilada funciona sin internet. Los requisitos, la arquitectura y las etapas de migración están en [PLAN-ESCRITORIO.md](PLAN-ESCRITORIO.md).

## Guion para mostrar al cliente

1. Desde **Pacientes**, registra un paciente ficticio. Desde su ficha, selecciona **Agendar cita** y guarda un horario.
2. En **Agenda**, abre al paciente desde esa cita. Crea una atención, completa algunas respuestas del cuestionario y registra un hallazgo en la pieza 16. Guarda la atención.
3. En **Tratamientos y pagos**, añade un procedimiento de S/ 300. Abre la vista imprimible del consentimiento de demostración y, si quieres ensayar el adjunto, sube un PDF o una imagen ficticia.
4. Registra S/ 100 recibidos en efectivo. Revisa el saldo de S/ 200. Usa **Saldar pendiente** para registrar un segundo cobro de S/ 200.
5. Desde **Caja**, revisa ambos movimientos y la auditoría. Si anulas el último cobro con un motivo, el sistema vuelve a mostrar S/ 200 por cobrar.
6. Cierra sesión y vuelve a entrar para consultar lo guardado.

## Compilar desde el código

Se usan Go, Node.js y npm para compilar. El ejecutable final incluye la interfaz.

```powershell
powershell -ExecutionPolicy RemoteSigned -File .\scripts\compilar-escritorio.ps1
go test -tags desktop ./...
```

El consentimiento imprimible es una plantilla de demostración. La revisión del formato con BIODENTIS, el instalador y la recuperación de respaldos corresponden a las fases posteriores del plan de trabajo.


## Ajustes de la ficha · noviembre 2026

- El registro pide **edad en años** (opcional, 0–130), sin solicitar fecha de nacimiento. Es una edad declarada: puede actualizarse al editar la ficha. Para fichas antiguas con fecha de nacimiento se calcula la edad una vez durante la actualización y se conserva el dato original.
- Los signos vitales ya no aparecen en el formulario. Los valores previamente guardados se conservan.
- Antecedentes: **No** por defecto y **Sí** cuando corresponde. Al guardar se registran las doce respuestas. Las respuestas positivas existentes se mantienen; los campos antiguos vacíos se presentan como No y se normalizan al guardar.
- La ficha se organiza en **Atención clínica** y **Tratamientos y pagos**. Cambiar entre estos apartados no borra lo escrito.
- Al elegir un procedimiento aparece una opción para adjuntar una radiografía inicial. Ortodoncia fija incluye **brackets** y muestra automáticamente la sección de radiografías en el procedimiento guardado.
- Cada procedimiento puede guardar varios JPG, PNG o PDF (hasta 8 MB por archivo). Las imágenes y los PDF se abren en la ventana de la app. Los archivos se guardan en la base local, exigen sesión iniciada y generan un evento de auditoría. Los reintentos del mismo archivo no crean duplicados en el mismo procedimiento.
- El visor de esta demo permite consultar archivos exportados; no incluye DICOM, mediciones ni procesamiento de imágenes.
