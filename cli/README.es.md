# bearings

`bearings` es la herramienta de línea de comandos de Bearings. Guarda un
cuaderno de lo que aprendes en un rol nuevo: secciones y temas con un estado,
una bitácora con fechas y una **ruta del rol** que se convierte en guía para
quien ocupe el rol después.

El cuaderno es una carpeta de archivos Markdown sencillos. Se queda en tu
equipo, salvo que lo sincronices con un remoto de git, lo exportes o compartas
su ruta del rol. No hay cuentas, no hay llamadas de red propias y no hay
telemetría.

Idioma: español. [Read in English](README.md).

## Instalación

Necesitas Go 1.24.7 o posterior (ver `go.mod`), o una copia del código fuente.

Desde el proxy de módulos de Go:

```sh
go install github.com/IvanM0206/bearings/cli/cmd/bearings@latest
```

Desde una copia del repositorio:

```sh
git clone https://github.com/IvanM0206/bearings.git
cd bearings/cli
go build -trimpath -o bearings ./cmd/bearings
./bearings --help
```

Copia el binario a una carpeta de tu `PATH`. `bearings sync` necesita además
`git` 2.26 o posterior.

## Inicio rápido

```sh
# 1. Crea un cuaderno a partir de una plantilla integrada (en inglés o español).
bearings init --template trading-bots-developer-es --name "Mi primer rol"

# 2. Mira las secciones y su avance.
bearings ls

# 3. Mira una sección y luego un tema.
bearings ls tecnico
bearings show tecnico/entorno-local

# 4. Escribe una entrada de bitácora vinculada a un tema.
bearings log "Ejecuté el proyecto en mi equipo por primera vez" -t tecnico/entorno-local

# 5. Pasa un tema al siguiente estado: pendiente -> en curso -> dominado.
bearings advance tecnico/entorno-local

# 6. Mira la ruta del rol, la guía para quien ocupe el rol después.
bearings path
```

Los temas que vienen de una plantilla quedan marcados como *sugeridos* hasta
que los revises. `bearings ls <sección>` muestra la marca; `bearings topic edit
<ref> --clear-suggested` marca un tema como revisado.

Ejecuta `bearings init --list-templates` para ver las plantillas integradas.
Forman parte del programa y nunca se descargan.

Los ejemplos de esta guía usan la plantilla `trading-bots-developer-es`. Si
usas otra plantilla, cambia las referencias por las de su cuaderno (`bearings
ls` las muestra).

## Conceptos

- **Cuaderno**: un directorio con un archivo `bearings.yaml`, una carpeta
  `sections/` y una carpeta `journal/`.
- **Sección**: un área grande de tu rol (por ejemplo "Técnico" o "Equipo").
  Su nombre en los comandos es un *slug*, que se deriva del título al crearla
  y nunca cambia después.
- **Tema**: algo que necesitas aprender, dentro de una sección. Se indica como
  `<sección>/<tema>`, por ejemplo `tecnico/backtesting`.
- **Estado**: `pending` (pendiente), `in-progress` (en curso) o `mastered`
  (dominado).
- **Tipo**: `role` (parte de la ruta del rol, por defecto) o `personal` (fuera
  de la ruta del rol y de las guías compartidas).
- **Entrada de bitácora**: una nota con fecha. Puede vincularse a una sección o
  a un tema.
- **Ruta del rol**: todos los temas `role`, en orden, numerados a través de las
  secciones.

## Dónde está el cuaderno

El directorio del cuaderno se elige en este orden:

1. `--notebook <dir>`
2. la variable de entorno `BEARINGS_NOTEBOOK`
3. `~/bearings`

`init` lo crea, y necesita un directorio vacío o inexistente: nunca crea un
cuaderno sobre archivos existentes.

## Idioma

Los mensajes están disponibles en inglés (`en`, por defecto) y en español
(`es`). El idioma se toma de la primera de estas fuentes que esté definida:

1. `--lang en|es`
2. la variable de entorno `BEARINGS_LANG`
3. la clave `language` del `bearings.yaml` del cuaderno
4. la primera de las variables `LC_ALL`, `LC_MESSAGES` y `LANG` que esté
   definida; `en_US.UTF-8` o `es_AR` eligen su idioma, y una configuración
   regional sin catálogo (como `fr_FR`) cae en `en`
5. `en`

Un valor desconocido dado con `--lang` es un error. Los valores desconocidos
en `BEARINGS_LANG` o en la clave `language` del cuaderno se ignoran. El idioma
del cuaderno solo se aplica cuando no se definen `--lang` ni `BEARINGS_LANG`.
Los nombres de comandos, de flags y de archivos nunca se traducen.

## Comandos

Todos los comandos aceptan las opciones globales `--notebook`, `--lang`,
`--json` (en los comandos de lectura y de escritura, ver [salida JSON](docs/json-output.md)),
`--no-color` y `--help`. Los colores también se desactivan cuando `NO_COLOR`
está definido, cuando `TERM=dumb`, o cuando la salida no es una terminal.

### Cuaderno y secciones

| Comando | Qué hace | Ejemplo |
|---|---|---|
| `init` | Crea un cuaderno en blanco o desde una plantilla. `--name`, `--language`, `--template`, `--list-templates`. | `bearings init --template trading-bots-developer-es --language es` |
| `section add <título>` | Agrega una sección después de las existentes. `--description`. | `bearings section add "Clientes" --description "Trabajo con clientes"` |
| `section rename <sección> <título>` | Cambia el título de una sección; su slug no cambia. | `bearings section rename clientes "Cuentas"` |
| `section mv <sección> --up\|--down` | Mueve una sección en el orden. | `bearings section mv clientes --up` |
| `section rm <sección> [--yes]` | Elimina una sección y sus temas. Las entradas de bitácora conservan su texto pero pierden el vínculo. | `bearings section rm clientes --yes` |
| `section ls` | Igual que `bearings ls`. | `bearings section ls` |

### Temas

| Comando | Qué hace | Ejemplo |
|---|---|---|
| `topic add <sección> <título>` | Agrega un tema (de tipo role y pendiente por defecto). `--type personal`, `--status`. | `bearings topic add equipo "Calendario de releases" --type role` |
| `topic edit <sección>/<tema>` | Cambia `--title`, `--type`, `--clear-suggested`, `--add-resource <url>` con `--resource-title`, o `--remove-resource <url>`. | `bearings topic edit equipo/calendario-de-releases --add-resource https://example.com/calendario --resource-title "Calendario"` |
| `topic mv <ref> --up\|--down\|--to-section <s>` | Reordena un tema, o lo mueve al final de otra sección. | `bearings topic mv equipo/calendario-de-releases --up` |
| `topic rm <ref> [--yes]` | Elimina un tema y sus recursos. | `bearings topic rm equipo/calendario-de-releases --yes` |
| `topic ls <sección> [--status] [--type]` | Igual que `bearings ls <sección>`. | `bearings topic ls tecnico --status pending` |
| `advance <ref>` | Pasa al siguiente estado; un tema dominado sigue dominado. | `bearings advance tecnico/backtesting` |
| `status <ref> <pending\|in-progress\|mastered>` | Define el estado directamente. | `bearings status tecnico/backtesting mastered` |

### Lectura

| Comando | Qué hace | Ejemplo |
|---|---|---|
| `ls` | Secciones con el número de temas dominados. | `bearings ls` |
| `ls <sección> [--status] [--type]` | Temas de una sección, con estado, tipo y si son sugeridos. | `bearings ls tecnico --type role` |
| `show <ref>` | Un tema con sus recursos, notas y entradas de bitácora vinculadas. | `bearings show tecnico/backtesting` |
| `search <consulta> [--limit n]` | Búsqueda sin distinguir mayúsculas (ignora acentos) en secciones, temas, recursos y la bitácora. | `bearings search backtesting --limit 10` |
| `journal [--from] [--to] [-s sección] [--limit n]` | Entradas de bitácora, de la más reciente a la más antigua, agrupadas por fecha. Las fechas son `YYYY-MM-DD`, ambos límites incluidos. | `bearings journal --from 2026-01-01 --section tecnico` |
| `progress` | Temas dominados por sección y en total, además de los temas sugeridos aún sin revisar. | `bearings progress` |
| `path [--md]` | La ruta del rol. `--md` imprime una guía en Markdown con los temas del rol, sus notas y recursos, y sin progreso personal. | `bearings path --md > guia-del-rol.md` |

`path --md` escribe las notas como código Markdown. El HTML sin procesar que
aparezca en las notas no se escapa ahí, así que revisa la guía, o muéstrala con
un visor que desactive el HTML sin procesar, antes de publicarla.

### Bitácora

| Comando | Qué hace | Ejemplo |
|---|---|---|
| `log [texto\|-] [-s sección] [-t sección/tema] [--date YYYY-MM-DD]` | Agrega una entrada. Da el texto como argumento, `-` para leerlo de la entrada estándar, o no des texto para escribirlo en tu editor. | `echo "Subí el primer cambio" \| bearings log - -t tecnico/entorno-local --date 2026-11-04` |
| `edit <sección>/<tema>` | Edita las notas de un tema en tu editor (`$VISUAL` o `$EDITOR`). | `bearings edit tecnico/backtesting` |
| `edit --journal <id>` | Edita el texto de una entrada de bitácora. El id se muestra con `show` y con `journal`. | `bearings edit --journal g85qj7ggwk` |

`edit` y `log` sin texto usan un archivo temporal privado (modo `0600`, en un
directorio nuevo) que se elimina al terminar. Si el archivo del cuaderno cambió
mientras `edit` estaba abierto, no se sobrescribe nada: tu texto se conserva en
un archivo de recuperación, y se muestra su ruta.

### Exportar, importar y compartir

| Comando | Qué hace | Ejemplo |
|---|---|---|
| `export <archivo.zip> [--force] [--role-only]` | Escribe el cuaderno en un archivo ZIP que pueden leer `import` y la aplicación web. `--role-only` deja fuera la bitácora y los temas personales, y marca cada tema restante como pendiente y sugerido: así se obtiene una guía para compartir. | `bearings export ~/guia-del-rol.zip --role-only` |
| `import <archivo.zip> [--into dir] [--yes]` | Crea un cuaderno nuevo a partir de un archivo. Nunca combina ni sobrescribe un cuaderno. | `bearings import ~/guia-del-rol.zip --into ~/nuevo-rol --yes` |

`export` solo incluye los archivos propios del cuaderno (`bearings.yaml`,
`sections/` y `journal/`), así que un directorio `.git` u otros archivos de la
carpeta quedan afuera. El archivo no puede escribirse dentro del cuaderno, y un
archivo existente solo se reemplaza con `--force`.

`import` revisa el archivo completo antes de escribir nada. Si lo rechaza, no
se crea nada. El cuaderno nuevo va a `--into`, o si no a la ubicación del
cuaderno, que debe estar vacía o no existir.

### Sincronización

| Comando | Qué hace | Ejemplo |
|---|---|---|
| `sync [--dry-run] [--verbose]` | Confirma en git los archivos del cuaderno que cambiaron. Si la rama tiene upstream, después hace pull con rebase y push. | `bearings sync --dry-run` |

Consulta [docs/sync.es.md](docs/sync.es.md) para la configuración y el modelo
de seguridad completo de `sync`. En resumen, el directorio del cuaderno debe
ser el nivel superior de su propio repositorio git, y el remoto debe ser
privado.

```sh
git -C ~/bearings init
bearings sync
git -C ~/bearings remote add origin <url de un repositorio privado>
git -C ~/bearings push -u origin HEAD
```

`bearings sync` nunca resuelve conflictos, no reinicia, no guarda cambios en
stash y no hace push forzado. Ante un conflicto, se detiene y muestra los
comandos de git para revisarlo.

### Autocompletado de la shell

```sh
bearings completion bash > /etc/bash_completion.d/bearings   # o zsh, fish, powershell
```

## Confirmación y scripts

Los comandos que eliminan datos (`section rm`, `topic rm`, `topic edit
--remove-resource`) y `import` piden confirmación en una terminal. Sin terminal,
terminan con código de salida 2, salvo que se dé `--yes` (`-y`), así que un
script debe decirlo a propósito.

## Códigos de salida

| Código | Significado |
|---|---|
| 0 | Éxito |
| 1 | Fallo inesperado (por ejemplo, un error de entrada/salida o un conflicto con un cambio hecho en otro lugar) |
| 2 | Error de uso: un flag, argumento, referencia, fecha o consulta no válidos, o una confirmación que no se dio |
| 3 | El cuaderno no existe o no es válido |
| 4 | La sección o el tema no existe |

Los mensajes van a stderr; los resultados, a stdout.

## Salida JSON

Los comandos de lectura y de escritura aceptan `--json`. La salida es un objeto
JSON, y las advertencias van a stderr. Las formas están documentadas en
[docs/json-output.md](docs/json-output.md). Los campos de texto son el contenido
sin procesar del cuaderno: trátalos como datos no confiables.

## Modelo de seguridad

`bearings` trata el cuaderno como entrada no confiable, porque puede venir de la
plantilla, el archivo o el repositorio git de otra persona. Las reglas siguientes
son las de las secciones 7 y 8 de [format/SPEC.md](../format/SPEC.md), tal como
las implementa la CLI.

**Archivos y rutas**

- La raíz del cuaderno se resuelve una sola vez a una ruta absoluta sin
  enlaces simbólicos. Cada ruta que abre la CLI se construye a partir de slugs,
  fechas e ids validados, nunca de la entrada sin procesar, y se comprueba que
  quede dentro de la raíz.
- Los enlaces simbólicos nunca se siguen dentro del cuaderno. Los enlaces y los
  archivos especiales se omiten con una advertencia, y `import` rechaza los
  archivos que los contengan.
- Cada escritura es atómica: el contenido nuevo va a un archivo temporal en el
  mismo directorio, se hace `fsync` y luego se renombra sobre el archivo
  anterior. Si una escritura falla, el archivo anterior queda como estaba.
- Los archivos nuevos se crean con modo `0600`, y los directorios nuevos con
  modo `0700`.
- Antes de sobrescribir un archivo, la CLI comprueba que no haya cambiado desde
  que se leyó. Si cambió, rechaza la escritura y conserva tu texto en un archivo
  de recuperación.
- La CLI nunca abre un archivo que no sea una de las rutas de diseño de la
  especificación. Los demás archivos de la carpeta (`.git`, archivos de
  intercambio del editor y similares) se ignoran.

**Límites** (todo lector los aplica y, al alcanzarlos, indica el límite)

| Límite | Valor |
|---|---|
| Tamaño de archivo (cualquier `.md` o `bearings.yaml`) | 1 MiB |
| Frontmatter | 16 KiB |
| Secciones | 500 |
| Temas por sección | 2 000 |
| Entradas de bitácora | 200 000 |
| Profundidad de directorios bajo la raíz | 4 |

El frontmatter y el `bearings.yaml` se leen con un subconjunto estricto de
YAML. Se rechazan los anclajes, los alias, las etiquetas, las claves de fusión,
los documentos múltiples, las claves duplicadas, las claves desconocidas y los
valores de tipo incorrecto.

**Archivos comprimidos** (`import`): se revisan por completo antes de escribir
nada. Se rechaza un archivo que tenga más de 210 000 entradas, más de 200 MiB de
datos sin comprimir, una entrada más grande de lo que puede ser un archivo del
cuaderno, una ruta absoluta, un segmento `..`, una barra invertida, una letra de
unidad, un segmento vacío o un byte NUL en un nombre, un enlace simbólico, un
dispositivo u otra entrada especial, dos entradas con el mismo nombre tras
normalizarlo, una razón de compresión superior a 100:1, una entrada cifrada o un
método de compresión no compatible. Los datos se cuentan mientras se descomprimen,
no solo a partir de los tamaños declarados.

**Salida**

- El contenido del cuaderno se imprime sin caracteres de control ni secuencias
  de escape ANSI, así que una nota no puede cambiar tu terminal.
- Los mensajes de error indican el archivo, el campo o el límite, y nunca
  imprimen el contenido de los archivos.
- No se usan colores cuando la salida no es una terminal, o cuando `NO_COLOR`
  está definido.

**Git (`sync`)**

- git siempre se ejecuta con una lista de argumentos explícita, nunca a través
  de una shell, y con `-C <cuaderno>`. Ningún contenido del cuaderno es un
  argumento: el mensaje de confirmación es fijo, y las rutas a confirmar se
  envían a git por la entrada estándar.
- Los hooks, fsmonitor, la recursión en submódulos y el transporte `ext::` se
  desactivan en cada comando de git. Los editores y paginadores nunca se
  ejecutan.
- Un repositorio cuya propia configuración define un ajuste que ejecuta
  programas o envía tus datos a otro lugar (por ejemplo `core.sshCommand`, un
  auxiliar de credenciales, un filtro o una reescritura `insteadOf`) se rechaza,
  indicando los nombres de los ajustes. `sync` lee solo los nombres de los
  ajustes, nunca sus valores.
- `sync` nunca maneja credenciales: los auxiliares de git y el agente SSH
  funcionan como siempre.

**Red y datos**

- La CLI no hace llamadas de red propias. Solo `bearings sync` se comunica con
  un remoto, y únicamente cuando tú lo ejecutas y la rama tiene upstream.
- Las plantillas están incluidas en el binario. Nunca se descargan.
- No se envía nada a ningún otro lugar: no hay telemetría ni analítica.

Si encuentras un problema de seguridad, repórtalo en privado, como se describe
en [SECURITY.md](../SECURITY.md).

## Desarrollo

```sh
cd cli
go test -race ./...
go vet ./...
golangci-lint run
govulncheck ./...
```

Los archivos golden con la salida de los comandos están en
`internal/cmd/testdata/golden`, en inglés y en español. Vuelve a generarlos después de
un cambio intencional de salida con `go test ./internal/cmd -run Golden -update`,
y revisa el diff.
