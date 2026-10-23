# VPN-UI v1.0.0: primera versión estable

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Bahasa Indonesia](README_ID.md) | [日本語](README_JA.md) | [Português (Brasil)](README_PT_BR.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [Українська](README_UK.md) | [Tiếng Việt](README_VI.md) | [简体中文](README_ZH.md) | [繁體中文](README_ZH_TW.md)

VPN-UI es un panel de gestión de VPN multiprotocolo. **v1.0.0 es la primera versión estable de esta bifurcación.** La fuente, los instaladores y las actualizaciones provienen de [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui), basado en [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) y [3X-UI](https://github.com/MHSanaei/3x-ui).

## Trece idiomas

El panel, la página del navegador de suscripción, el bot de Telegram y los controles de interfaz de usuario compartidos admiten inglés, persa, árabe, español, indonesio, japonés, portugués brasileño, ruso, turco, ucraniano, vietnamita, chino simplificado y chino tradicional. Elija el idioma web en el selector de idiomas; configure el idioma del bot en la configuración de Telegram. El bot usa el idioma configurado, no una preferencia automática separada para cada chat de Telegram.

Las solicitudes web mantienen su propio idioma, incluidas las solicitudes simultáneas de panel y suscripción. Las variantes regionales se resuelven en configuraciones regionales admitidas. Los diálogos, calendarios y paginación utilizan el idioma seleccionado. Los identificadores de protocolo, las claves de configuración, los nombres de comandos y los resultados de diagnóstico conservan su sintaxis original. Los instaladores de terminales utilizan el inglés; el asistente de migración ofrece persa e inglés. Consulte [docs/TRANSLACIONES.md](docs/TRANSLATIONS.md).

## Protocolos y núcleos

| Tiempo de ejecución | Protocolos e implementaciones |
| --- | --- |
| Rayos X incluidos | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, AnyTLS, TUIC, NaiveProxy, WireGuard |
| Backends VPN portátiles | OpenVPN (TCP/UDP), OpenConnect (ocserv), L2TP, PPTP, SSTP e IKEv2 |
| Túneles de núcleo | WireGuard (`wg-c`), AmneziaWG (`awg`) y GRE, con IPsec o FOU opcional cuando sea compatible |
| Relés incorporados | Proxy MTProto y proxy SSH con autenticación por cuenta |

IPsec se puede seleccionar en la instalación principal y también es una dependencia compartida de L2TP/IKEv2. Las dependencias necesarias se conservan mientras otro núcleo instalado las necesita. Kernel WireGuard y Xray WireGuard nativo son implementaciones diferentes. La función SSH incorporada reenvía el tráfico VPN; no proporciona un shell raíz.

Los binarios completos incluyen backends portátiles, el núcleo Xray parcheado, un cliente ACME y seis archivos de geodatos: base, geoip/geosite de Irán y Rusia. Los paquetes del sistema operativo, los módulos del kernel, los encabezados del kernel y DKMS siguen siendo requisitos del host. La página Núcleos muestra el progreso de la instalación, los requisitos previos que faltan y el estado real del servicio. El GRE simple no está cifrado a menos que IPsec esté habilitado; PPTP es un protocolo heredado. Consulte [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Usuarios, grupos y suscripciones

Administre clientes en **Usuarios**, infraestructura en **Entradas** y asignaciones entrantes reutilizables en **Grupos**. Una cuenta puede utilizar varias entradas y ubicaciones compatibles con una cuota y suscripción compartidas. Los cambios de grupo actualizan las asignaciones sin reemplazar las credenciales de la cuenta. Los cambios de membresía individuales y el cambio de cuenta global permanecen separados.

Los límites de tráfico se almacenan en bytes a pesar del nombre del campo API `totalGB`. Una cuenta con límite de tráfico cero tiene tráfico ilimitado; una expiración de cero significa tiempo ilimitado. Los planes de inicio retrasado comienzan con el primer uso. Los totales de descripción general cuentan cuentas únicas, no un usuario por cada membresía. Compartir una ID de suscripción entre distintas cuentas publica sus configuraciones juntas; no fusiona sus identidades ni convierte sus límites separados en una cuota de cuenta compartida.

El núcleo incluido atribuye nuevo uso a su fuente entrante. La información de la cuenta muestra las acciones medidas y cualquier resto histórico no atribuido por separado. La actualización preserva el uso existente; Los registros antiguos de solo cuenta no pueden recrear una división histórica. Los puntos finales de proxy externo se utilizan de manera consistente en las exportaciones de cuentas/configuraciones. Los perfiles de OpenVPN y los archivos de pares de WireGuard se pueden descargar desde sus vistas de configuración correspondientes.

### Renovar, restablecer el tráfico y congelar

**Usuarios → el menú de tres puntos del cliente → Renovar** borra la carga/descarga actual entre sus membresías, inicia la duración completa del paquete a partir de ahora y habilita la cuenta. Se admiten planes de solo tráfico, solo de tiempo, combinados y totalmente ilimitados. La cuota, la contraseña, el UUID, el ID de suscripción, las claves de dispositivo, las direcciones de pares y el acceso entrante permanecen sin cambios. Las duraciones heredadas desconocidas solicitan el número de días una vez y lo recuerdan.

**Restablecer tráfico** borra el uso actual sin reiniciar la caducidad ni habilitar una cuenta deshabilitada manualmente. **Congelar** detiene los días restantes; la descongelación restaura el resto. En su lugar, la renovación utiliza la duración completa del paquete registrado. Se conservan la contabilidad de por vida y los contadores de nodos acumulativos. Aún se aplican límites representativos y membresías para discapacitados. Consulte [docs/CLIENT-RENEWAL.md](docs/CLIENT-RENEWAL.md) y [docs/PANEL-GUIDE.md](docs/PANEL-GUIDE.md).

### Límites de velocidad y acceso

Las anulaciones de descarga/carga aceptan KB/s, MB/s o Mbps. Vacío hereda la configuración de entrada; cero significa ilimitado en esa dirección. El almacenamiento utiliza KB/s de 1024 bytes, mientras que Mbps significa megabits decimales. Los MB/s de la interfaz de usuario utilizan 1024 bytes: **3 MB/s = 24 Mib/s ≈ 25.17 Mbps**. Los valores de velocidad se redondean a al menos 1 KB/s cuando son positivos.

Los límites de IP de origen cuentan las direcciones visibles, no los dispositivos físicos detrás de una NAT. Los límites de sesión y de pares dependen del protocolo. WireGuard/AmneziaWG emiten configuraciones de dispositivos independientes. Los límites admitidos se muestran en cada formulario; Los límites de sesión de nodo/IP son locales para cada nodo.

## Administradores y representantes

El administrador inicial es el propietario del panel protegido. Los roles de administrador y las concesiones personalizadas determinan las operaciones; Las asignaciones grupales/entrantes determinan su alcance. La base de datos sensible al propietario, la autenticación y las operaciones ejecutables mantienen una protección separada. Una concesión explícita de configuración del panel abre su página de configuración permitida completa, incluidos Telegram, LDAP, certificados y configuraciones de servicio.

Las nuevas cuentas representativas utilizan contabilidad basada en el consumo y roles de ámbito. Las cuentas prepagas existentes conservan sus propias reglas de facturación. Las concesiones de actualización, eliminación y restablecimiento de uso son independientes. La renovación manual requiere permisos de actualización y restablecimiento con un alcance más limitado. La transferencia de propiedad conserva las credenciales del cliente y está restringida a propietarios elegibles. Un representante cuya asignación se agote permanece sujeto a la política de desconexión configurada.

Cada cuenta que haya iniciado sesión puede administrar su propio autenticador en **Seguridad de la cuenta**. La inscripción requiere la contraseña actual y un código válido de un solo uso. Los gerentes pueden exigir inscripción de dos factores para los representantes; La inscripción pendiente solo permite la seguridad de la cuenta y el cierre de sesión. Los secretos y los enlaces de suscripción deben tratarse como credenciales.

## Nodos de múltiples ubicaciones

Un panel central administra agentes de ubicación dedicados a través de HTTPS saliente verificado. Un nodo está emparejado con un panel. Instale el agente en un servidor de ubicación independiente y luego seleccione los núcleos necesarios en el cuadro de diálogo Núcleos del nodo; La instalación inicial del agente no activa ningún núcleo VPN. Delegue la gestión de nodos con **Administrar nodos y ubicaciones**. Los representantes no pueden obtener esta subvención de infraestructura.

El sondeo de agentes y la página de nodos visible se actualizan cada segundo. Los diagnósticos del host se actualizan por separado, normalmente cada cinco segundos. Las instantáneas sin cambios evitan reescribir las tablas de clientes; La configuración modificada reconcilia los núcleos afectados. La instalación/eliminación principal, los controles de tiempo de ejecución y las ediciones de configuración muestran un progreso y reconocimientos persistentes.

Las cuentas conservan una cuota central y una suscripción en todas las ubicaciones. Los informes acumulativos aceptados no se facturan dos veces. Los cambios se propagan de forma asincrónica; El tráfico en vuelo y las ubicaciones simultáneas pueden superar brevemente una cuota. Los datos del nodo en línea quedan obsoletos después de 35 segundos. La pérdida de la concesión de administración durante 90 segundos detiene el plano de datos VPN mientras se retienen los contadores; una nueva instantánea válida restaura el servicio. Consulte [docs/NODES.md](docs/NODES.md) y [docs/NODES.fa.md](docs/NODES.fa.md).

## Requisitos del sistema

Se requiere Linux amd64 (x86-64) o arm64 (aarch64), acceso root, Bash y un sistema systemd en ejecución. Ubuntu 24.04 y Debian 12/13 son objetivos de implementación recomendados. Los privilegios de red del host y los módulos específicos del protocolo siguen siendo necesarios. Una instalación exitosa del panel no certifica todos los protocolos en cada distribución, kernel o VPS. Consulte [table.md](table.md).

El directorio de instalación estándar es `/opt/vpn-ui`. El ejecutable instalado mantiene el nombre de compatibilidad `/opt/vpn-ui/vpn-ui-amd64` en ambas arquitecturas. Los nombres de archivos principales incluidos siguen su arquitectura real. El nodo utiliza `/opt/vpn-ui-node` y su propio servicio systemd.

## Instalar o actualizar en línea

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

El mismo comando actualiza una instalación estándar existente conservando su base de datos y su configuración de inicio de sesión. Primero haz una copia de seguridad. Las instalaciones nuevas imprimen credenciales, puertos y rutas web aleatorios. Abra la URL exacta con el esquema HTTP/HTTPS impreso y reemplace una dirección de escucha como `0.0.0.0` con la dirección accesible del servidor. Las instalaciones nuevas utilizan HTTP hasta que se configura TLS.

Para un nodo emparejado existente, descargue y ejecute su instalador; elija instalar/actualizar en el menú. Se conservan los emparejamientos y contadores existentes:

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/install-node.sh -o install-node.sh
sudo bash install-node.sh
```

Un nuevo nodo debe utilizar el comando de emparejamiento generado en el panel central. El menú del nodo también proporciona estado, inicio, parada, reinicio, registros recientes y eliminación confirmada. `--install` mantiene la ruta de actualización directa sin menú. Consulte [management-cli-plan.md](management-cli-plan.md).

## Instalación y actualización sin conexión

Descargue el binario que coincide con la arquitectura, `install-offline.sh` y `SHA256SUMS` de la [versión estable](https://github.com/admin6501/vpn-ui/releases/tag/v1.0.0). Transfiérelos juntos y verifique los archivos disponibles antes de la instalación:

```bash
sha256sum --ignore-missing -c SHA256SUMS
sudo bash install-offline.sh ./vpn-ui-amd64
# ARM64: sudo bash install-offline.sh ./vpn-ui-arm64
```

La pregunta de requisito previo apto opcional tiene por defecto **No**, incluido el EOF. No mantiene la instalación del panel local; Sí, requiere repositorios aptos accesibles. Se informan fallas de apt y la instalación local continúa. Preinstale o transfiera los paquetes del sistema operativo y los módulos del kernel necesarios por separado. Las actualizaciones conservan la base de datos, las credenciales y la configuración de TLS y mantienen copias de seguridad privadas.

El instalador registra `VPNUI_OFFLINE=1`, deshabilitando las descargas automáticas de paquetes, la búsqueda de IP pública y las actualizaciones de geodatos. El tráfico VPN todavía funciona; Las actualizaciones manuales, la emisión/renovación WARP y ACME aún necesitan Internet. Para la instalación del nodo fuera de línea, utilice el binario del nodo coincidente y `install-node.sh` con sus opciones de emparejamiento. Consulte [docs/OFFLINE-INSTALL.md](docs/OFFLINE-INSTALL.md).

## Migración de respaldo de PasarGuard

Primero instale un panel de destino compatible. Sus cuentas VPN y sus entradas deben estar vacías; Se conservan las configuraciones web/TLS del panel existente. Ejecutar:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh)
```

Elija persa o inglés y proporcione la ruta ZIP de respaldo absoluta. El paquete de migración contiene scripts y configuración, no otro panel binario. Se requieren Python 3 y descomprimir. Para la conversión sin conexión, transfiera y extraiga `vpn-ui-auto-migration.zip`, luego ejecute su `migrate.sh` directamente con la ruta de respaldo.

Se importan las identidades de cuentas admitidas, la propiedad del administrador, los grupos/asignaciones, los hosts, los hashes de contraseñas y los metadatos de compatibilidad de suscripciones. Utilice la contraseña de administrador anterior; los hashes no pueden revelar la contraseña en texto plano. Los nodos PasarGuard no se importan. Un dominio/IP cambiado requiere actualizar la dirección base de la suscripción y verificar los puntos finales exportados. Revise la configuración convertida y pruebe los protocolos que realmente utiliza. Consulte [migration/INSTALL-MIGRATION.fa.md](migration/INSTALL-MIGRATION.fa.md).

## Operación, respaldo y recuperación.

```bash
sudo vpn-ui
systemctl status vpn-ui --no-pager
journalctl -u vpn-ui -n 100 --no-pager
```

Utilice los datos de inicio de sesión privados impresos o el menú de administración para recuperar el acceso. Detenga el servicio antes de copiar archivos SQLite y conserve los complementos WAL/SHM, o realice una copia de seguridad SQLite consistente. Conserve de forma segura el material TLS, las anulaciones de servicios y las bases de datos de nodos. Un restablecimiento o renovación del tráfico no es una restauración de la base de datos. La desinstalación elimina datos; inspeccione su confirmación antes de continuar. Consulte [accounts-upgrade-guide.md](accounts-upgrade-guide.md) y [SECURITY.md](SECURITY.md).

## Liberar activos

| Archivo | Propósito |
| --- | --- |
| `vpn-ui-amd64` | Panel central completo para Linux x86-64 |
| `vpn-ui-arm64` | Panel central completo para Linux aarch64 |
| `vpn-ui-node-amd64` | Agente de ubicación dedicado para Linux x86-64 |
| `vpn-ui-node-arm64` | Agente de ubicación dedicado para Linux aarch64 |
| `install-offline.sh` | Instalador/actualización del panel local con soporte de reversión |
| `install-node.sh` | Menú de instalación, actualización y gestión de nodos. |
| `vpn-ui-auto-migration.zip` | Configuración y scripts de conversión de PasarGuard |
| `SHA256SUMS` | Sumas de comprobación SHA-256 para los siete artefactos descargables anteriores |

## Construcción, pruebas y documentación.

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh --help
./build.sh
```

Utilice la cadena de herramientas Go de `go.mod` y los submódulos/parches de seguridad anclados. Una versión completa debe construir sus núcleos, backends y todos los datos geográficos; los paquetes omitidos producen binarios de desarrollo. La compilación requiere dependencias en línea, una cadena de herramientas C y herramientas de compilación de backend, incluido Docker cuando sea necesario. ACME necesita Internet incluso con su cliente incluido.

El flujo de trabajo de lanzamiento valida ambas arquitecturas nativas, contabilidad enfocada y comportamiento de permisos, rutas de datos de nodos/rayos X reales, instalación/actualización fuera de línea y comportamiento del navegador en los idiomas admitidos. La matriz de distribución/protocolo Incus separada requiere un host de VM privilegiado preparado e Internet; su presencia no es evidencia de que se hayan vuelto a ejecutar todas las entradas de la matriz. Consulte [test_unit/README.md](test_unit/README.md), [docs/INDEX.md](docs/INDEX.md), [api-reference.md](api-reference.md) y [.github/workflows/release.yml](.github/workflows/release.yml).

## Licencia

[GPL-3.0](LICENSE). Los avisos originales del proyecto y de terceros permanecen intactos. Los README suministrados/de submódulo describen sus propios componentes; Utilice las guías de esta horquilla para la instalación y operación del panel.
