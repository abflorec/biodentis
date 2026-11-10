# BIODENTIS local

Demo funcional para una clínica odontológica de una sede y un equipo. Incluye acceso con usuario y contraseña, agenda, pacientes, historia clínica, odontograma, procedimientos, consentimiento de prueba, radiografías, cobros y auditoría. El ejecutable sirve las pantallas y guarda los datos en SQLite en la misma PC.

## Abrir la demo

1. En Windows de 64 bits, haz doble clic en `ABRIR-DEMO.cmd`. El iniciador abre [BIODENTIS en el navegador](http://127.0.0.1:8090).
2. Ingresa con el usuario `biodentis.demo` y la contraseña temporal `BiodentisDemo2026!`.
3. Trabaja con datos ficticios. La demo crea la base `demo-data` en la primera ejecución y conserva los cambios entre sesiones. Si cambias la contraseña, seguirá vigente al volver a abrir la demo.

El paquete se entrega con `bin/BIODENTIS.exe` y funciona sin internet. Para iniciar en una terminal visible desde esta carpeta, ejecuta `.\bin\BIODENTIS.exe -demo -port 8090 -data demo-data` y abre la misma dirección en el navegador.

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
npm ci
npm run build
go test ./...
go build -trimpath -o bin/BIODENTIS.exe ./server
```

El consentimiento imprimible es una plantilla de demostración. La revisión del formato con BIODENTIS, el instalador con arranque automático, la recuperación de respaldos corresponden a las fases posteriores descritas en el plan de trabajo.


## Ajustes de la ficha · noviembre 2026

- El registro pide **edad en años** (opcional, 0–130), sin solicitar fecha de nacimiento. Es una edad declarada: puede actualizarse al editar la ficha. Para fichas antiguas con fecha de nacimiento se calcula la edad una vez durante la actualización y se conserva el dato original.
- Los signos vitales ya no aparecen en el formulario. Los valores previamente guardados se conservan.
- Antecedentes: **No** por defecto y **Sí** cuando corresponde. Al guardar se registran las doce respuestas. Las respuestas positivas existentes se mantienen; los campos antiguos vacíos se presentan como No y se normalizan al guardar.
- La ficha se organiza en **Atención clínica** y **Tratamientos y pagos**. Cambiar entre estos apartados no borra lo escrito.
- Al elegir un procedimiento aparece una opción para adjuntar una radiografía inicial. Ortodoncia fija incluye **brackets** y muestra automáticamente la sección de radiografías en el procedimiento guardado.
- Cada procedimiento puede guardar varios JPG, PNG o PDF (hasta 8 MB por archivo). Las imágenes se abren con zoom y los PDF en otra pestaña. Los archivos se guardan en la base local, exigen sesión iniciada y generan un evento de auditoría. Los reintentos del mismo archivo no crean duplicados en el mismo procedimiento.
- El visor de esta demo permite consultar archivos exportados; no incluye DICOM, mediciones ni procesamiento de imágenes.
