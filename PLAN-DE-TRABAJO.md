# Plan de trabajo de BIODENTIS local

## Objetivo y alcance

Preparar una demo funcional del sistema interno de BIODENTIS para una sede y una PC con Windows. La persona que administra la clínica también atiende, agenda y cobra. Usará una sola cuenta con nombre de usuario y contraseña y podrá pasar de una tarea a otra durante la atención.

La demo usará pacientes ficticios y se ejecutará en el navegador de la PC con internet desconectado. El caso completo debe quedar guardado al cerrar y volver a abrir el sistema. La demo incluye adjuntos JPG, PNG y PDF por procedimiento, con ampliación de imágenes.

## Flujo de la demo

1. Entrar con un usuario como `biodentis.demo` y su contraseña de demostración.
2. Registrar al paciente ficticio, asignarle un número de historia clínica y agendarle una cita.
3. Abrir la atención desde la agenda. Completar parte del cuestionario y del examen y registrar un hallazgo en la pieza 16.
4. Agregar un procedimiento al plan, emitir un presupuesto de S/ 300 y abrir la vista imprimible del consentimiento.
5. Adjuntar un consentimiento ficticio previamente firmado y comprobar que queda vinculado al tratamiento correspondiente.
6. Registrar un abono de S/ 100 en efectivo para el procedimiento de S/ 300. Comprobar que queda **Parcial** con S/ 200 pendientes y consultar el movimiento desde caja y desde la ficha.
7. Simular una visita posterior. Una vez recibidos los S/ 200 en efectivo, usar **Saldar pendiente** y verificar que hay dos movimientos y que el procedimiento queda **Pagado**.
8. Cerrar el navegador, iniciar sesión de nuevo y verificar la cita, el odontograma, el consentimiento y los dos cobros guardados.

## Pantallas y funciones

| Pantalla | Primera versión |
| --- | --- |
| Inicio de sesión | Nombre de usuario y contraseña, cierre de sesión y cambio de contraseña. La cuenta propietaria tiene acceso a toda la operación de la clínica. |
| Agenda y pacientes | Lista de citas del día. Alta y búsqueda por nombre o número de historia. Creación, reprogramación y cancelación de citas. Acceso directo a la atención desde una cita. |
| Historia clínica | Datos de identificación y contacto, edad declarada, cuestionario de antecedentes, examen, diagnóstico, pronóstico y observaciones. Historial de atenciones por paciente. |
| Odontograma | Dentición temporal y permanente según la numeración del formato de BIODENTIS. Selección de pieza y registro de un hallazgo básico asociado a la atención. |
| Tratamiento y presupuesto | Catálogo inicial basado en la hoja de BIODENTIS. Cada procedimiento puede vincularse a una pieza cuando corresponda. El plan contiene procedimientos y sesiones. Cada procedimiento tiene su propio importe y saldo. |
| Actividades y consentimiento | Historial con fecha, pieza, diagnóstico y tratamiento, más laboratorio, observaciones y odontólogo responsable. Vista imprimible del consentimiento, estado pendiente o firmado y adjunto de prueba asociado al plan. |
| Caja | Abonos por procedimiento y acción **Saldar pendiente**. Cada cobro guarda importe, medio, fecha y usuario. La vista muestra el saldo y los cobros del día. Una anulación conserva el movimiento y registra el motivo, la fecha y el usuario. |

## Reglas que salen de las fichas de BIODENTIS

- El cuestionario usa **Sí** y **No**; No es el valor predeterminado. Se mantienen las respuestas positivas existentes. Los signos vitales salen de la interfaz y el registro pide edad, sin fecha de nacimiento.
- El examen incluye pulso, frecuencia respiratoria, presión arterial, temperatura bucal y peso, además del examen extraoral e intraoral. La ficha conserva la fecha y el responsable de la atención.
- **A/cta.** representa el importe abonado al procedimiento. Un procedimiento puede quedar pendiente mientras dura el tratamiento, recibir varios abonos o pagarse completo en una sola visita, incluso al finalizar.
- La columna **Firma** de actividades se revisará con BIODENTIS para saber si deja constancia del cobro o si el paciente confirma la atención. El consentimiento y la firma del odontólogo se conservan como registros clínicos separados de caja.
- La ficha conserva el registro de las atenciones previas. Las correcciones y anulaciones sensibles quedan identificadas con usuario y fecha.

## Pagos y firma

- **Registrar abono.** El operador selecciona el procedimiento y registra lo que realmente recibió. El saldo es el importe acordado menos la suma de los pagos vigentes. El estado se calcula a partir de esos movimientos y se muestra como **Pendiente**, **Parcial** o **Pagado**.
- **Saldar pendiente.** El botón completa el importe que falta y pide confirmación de que se recibió el dinero. Usa el mismo registro que un abono, por lo que también sirve para cobrar el total de una vez. La finalización clínica y el estado de pago se controlan por separado.
- **Constancia de cobro.** Para la demo, el usuario que inició sesión confirma **Recibí el efectivo**. El sistema guarda el número del movimiento, el procedimiento, el importe, el medio, la fecha y el usuario. Si hay un error, se anula con motivo y se registra el movimiento correcto. El historial conserva el cobro original y la anulación.
- **Firma del paciente.** Si la columna de la ficha se usa para que el paciente confirme el pago, proponer una constancia interna imprimible con el importe y saldo para que pueda firmarla. Si la firma confirma la atención, conservar esa confirmación en la actividad clínica. Para el consentimiento informado, usar el formulario impreso firmado y adjuntarlo a la ficha. [Norma de gestión de la historia clínica del MINSA](https://www.gob.pe/es/institucion/minsa/normas-legales/187487-214-2018-minsa)

## Infraestructura propuesta

- **Aplicación local.** Go sirve la interfaz y las operaciones del sistema desde `127.0.0.1`. TypeScript y Vite se usan para construir la interfaz, que se empaqueta con los recursos de la aplicación para funcionar sin internet.
- **Datos.** Una base SQLite en el disco de la PC. Los pacientes, citas, atenciones, planes y procedimientos se vinculan por identificadores. Cada abono referencia al procedimiento que se está pagando. Las migraciones del esquema se versionan y los montos se guardan en céntimos. El registro del abono y la validación del saldo ocurren en una misma transacción, con protección ante reenvíos.
- **Acceso.** Una cuenta propietaria en la demo. Contraseña almacenada mediante hash, sesión con expiración y verificación de acceso en el servidor. La estructura de cuentas permitirá incorporar usuarios individuales si la clínica contrata personal.
- **Archivos y respaldos.** Los adjuntos quedan fuera de la carpeta pública y se consultan a través de la aplicación con sesión iniciada. Para el piloto se configurará una copia diaria cifrada de base y archivos en otro medio. Se probará la recuperación conjunta antes de ingresar datos reales.
- **Instalación para el piloto.** Paquete para Windows, acceso directo, arranque automático y carpeta de datos separada del ejecutable. Las actualizaciones tendrán respaldo previo y una prueba de la migración.

## Fases de implementación

| Fase | Qué se entrega | Criterio de cierre |
| --- | --- | --- |
| 1. Validación de las fichas | Flujo de pantallas, campos obligatorios y significado de las firmas. | BIODENTIS confirma quién firma cada parte del formato y qué acredita. |
| 2. Acceso, pacientes y agenda | Inicio de sesión, ficha de paciente y agenda local. | Se crea un paciente, se agenda una cita y se detecta un cruce de horario. |
| 3. Atención y tratamiento | Ficha clínica, odontograma, catálogo inicial y plan. | Desde la cita se registra un hallazgo por pieza y se guarda un procedimiento que puede volver a consultarse. |
| 4. Consentimiento y caja | Vista imprimible, adjunto de consentimiento, presupuesto, abonos por procedimiento, acción para saldar y resumen de caja. | Probar el caso de S/ 300 con dos cobros de S/ 100 y S/ 200, y otro procedimiento pagado de una vez. Repetir un envío conserva un solo cobro. Anular un abono devuelve el importe al saldo. |
| 5. Demo funcional | Paquete ejecutable con datos ficticios y guion de presentación. | El recorrido se completa sin internet. Después de reiniciar la aplicación se recuperan todos los datos del caso. |
| 6. Piloto con datos reales | Instalación en la clínica, credenciales definitivas y respaldo cifrado. | Se comprueba el arranque tras reiniciar Windows y se recuperan base y adjuntos desde una copia en un entorno de prueba. |

## Radiografías: primera versión incorporada

- Sección por procedimiento, desplegada automáticamente en ortodoncia/brackets. Permite adjuntar el primer archivo al crear el tratamiento y añadir más después.
- JPG y PNG con visor ampliable; PDF en una pestaña nueva. Límite de 8 MB por archivo, verificación del contenido, acceso con sesión y auditoría. Los archivos forman parte de la base SQLite y de su copia de seguridad.
- Se verifican persistencia, asociación al procedimiento, ausencia de duplicados al reenviar y denegación sin sesión.
- Pendiente para el piloto: confirmar el formato que exporta el equipo de la clínica; DICOM y herramientas de medición requieren otra fase. La restauración integral de respaldos sigue pendiente del piloto.

## Puntos por confirmar antes de comenzar

- Fecha de presentación y PC en la que se mostrará la demo.
- Campos obligatorios de la ficha y procedimientos que la clínica quiere ver primero.
- Quién firma la columna **Firma** de actividades y si esa firma acredita el cobro, la atención realizada o alguna otra confirmación.
