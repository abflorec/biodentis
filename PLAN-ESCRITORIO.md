# BIODENTIS Escritorio · Plan de trabajo

Convertir BIODENTIS en una aplicación de Windows que se abra con doble clic, funcione sin internet y conserve el flujo de agenda, pacientes, atención clínica, tratamientos, radiografías, consentimientos y caja.

## Primera versión

El ejecutable contiene Go y la interfaz compilada. Microsoft Edge WebView2 muestra esa interfaz en una ventana propia. El servicio clínico arranca dentro del mismo proceso en una dirección privada de la PC y se cierra al salir de la ventana. SQLite conserva los datos. Esta arquitectura permite migrar la presentación y la instalación reutilizando las reglas y la auditoría que ya se probaron.

- **Para probarla:** Windows de 64 bits y Microsoft Edge WebView2 Runtime. Abre `ABRIR-ESCRITORIO.cmd` o `bin/BIODENTIS-Escritorio.exe`. El usuario de esta demo es `biodentis.demo` y la contraseña inicial es `BiodentisDemo2026!`.
- **Datos:** la base demo de escritorio nace en `%LOCALAPPDATA%\BIODENTIS\EscritorioDemo\datos`. El perfil de la ventana y el registro de arranque se guardan junto a ella. La opción `-data <carpeta>` permite ensayar en otro perfil demo. Se permite una ventana por perfil.
- **Para compilar:** Go 1.26 o superior, Node.js compatible con el Vite del proyecto y npm. Ejecuta `powershell -ExecutionPolicy RemoteSigned -File .\scripts\compilar-escritorio.ps1`. El equipo que usa el ejecutable solo necesita el runtime de WebView2. Si falta, Microsoft ofrece un instalador completo para equipos sin conexión.

## Etapas de migración

| Etapa | Entrega | Criterio de cierre |
| --- | --- | --- |
| 1. Ventana y demo | Ejecutable con ventana propia, perfil aislado, acceso local y cierre del servicio al salir. | Abrir, iniciar sesión, registrar una atención, cerrar y reabrir con los datos guardados. |
| 2. Compatibilidad de módulos | Revisar agenda, fichas, odontograma, pagos y auditoría en WebView2. Verificar adjuntos, selector de archivos, visor PDF e impresión de consentimiento. | Completar el recorrido con un paciente ficticio, validar el saldo tras un abono y abrir los adjuntos correctos. Probar impresión en la PC de destino. |
| 3. Instalación y recuperación | Preparar instalador por usuario, icono, acceso directo, versión visible y diagnóstico de arranque. Agregar configuración inicial para la cuenta real y respaldos de SQLite, incluidos los adjuntos. | Instalar en una PC limpia, abrir sin internet y restaurar una copia en una carpeta de prueba. |
| 4. Traslado y piloto | Crear una copia consistente de la base que la clínica autorice. Verificar esquema, cantidades de registros, saldos y archivos. Ensayar primero en un perfil separado y conservar el respaldo previo al traslado. | La clínica valida la ficha y los documentos con una prueba completa. El cambio de equipo se programa una vez verificada la restauración. |

## Estado de la demo

En Windows se probó el recorrido de un paciente ficticio: inicio de sesión, cita, antecedentes, diagnóstico, brackets con radiografía, abono, liquidación y consulta en Caja. Se abrieron imágenes y PDF de varias páginas dentro de la ventana. Después de cerrar y volver a abrir el ejecutable se recuperaron la cita, los cobros y los adjuntos. Falta probar con la impresora y archivos de ejemplo del equipo de destino.

La primera compilación usa el modo demo con datos ficticios. La configuración real, el instalador y el traslado de datos son entregas posteriores. La versión web se puede seguir compilando desde este repositorio con el comando habitual, sin la etiqueta `desktop`.

Referencias técnicas: [Distribución de WebView2 Runtime](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/distribution), [enlace de Go para WebView2](https://github.com/jchv/go-webview2) y [PDF.js](https://mozilla.github.io/pdf.js/).
