# Servidor MCP de Bearings

*English: [mcp.md](mcp.md)*

`bearings mcp` ejecuta un servidor local de [Model Context Protocol](https://modelcontextprotocol.io)
para que un asistente de IA (Claude Code, Claude Desktop o cualquier cliente
MCP) pueda leer tu cuaderno y, si lo permites, agregarle contenido.

```
bearings mcp [--allow-write] [--log-level error|warn|info|debug] [--notebook DIR]
```

- **Solo stdio.** El asistente inicia `bearings mcp` como proceso hijo y se
  comunica con él por la entrada y la salida estándar. El servidor no abre
  ningún puerto de red ni hace llamadas de red.
- **Un cuaderno, fijo al iniciar.** Se elige igual que en los demás comandos:
  `--notebook`, luego `BEARINGS_NOTEBOOK` y luego `~/bearings`. Ninguna
  herramienta recibe una ruta de archivo; reciben slugs de sección
  (`technical`), referencias de tema (`technical/backtesting`), fechas
  (`2026-11-03`) y cursores, todos validados.
- **Solo lectura por defecto.** Las herramientas de escritura existen solo con
  `--allow-write`. No hay herramientas para borrar, mover ni renombrar.
- **Los registros van a stderr** (stdout lleva el protocolo), con el nivel de
  `--log-level` (por defecto `warn`). Contienen nombres de herramientas,
  duraciones, códigos de error y rutas dentro del cuaderno, nunca el contenido
  de las notas.

## Herramientas

Todas las listas están paginadas: cuando un resultado trae `next_cursor`,
vuelve a llamar a la herramienta con `cursor` igual a ese valor. Cada página
tiene como máximo 100 elementos (50 por defecto; se cambia con `limit` donde
existe) y unos 64 KiB de JSON.

### Herramientas de lectura (siempre disponibles)

| Herramienta | Entradas | Devuelve |
|---|---|---|
| `list_sections` | `cursor`, `limit` | Las secciones en orden: slug, título, orden y descripción. |
| `list_topics` | `section`, `status` (`pending`, `in-progress`, `mastered`), `type` (`role`, `personal`), `cursor`, `limit` | Temas sin notas: referencia, título, estado, tipo, si es sugerido y cantidad de recursos. |
| `get_topic` | `topic` (obligatorio), `notes_offset` | Un tema con sus recursos y notas. Las notas largas llegan en partes de unos 64 KiB: si `notes_truncated` es true, vuelve a llamar con `notes_offset` = `notes_next_offset`. |
| `search` | `query` (obligatorio, de 1 a 200 caracteres), `cursor`, `limit` | Coincidencias en títulos y textos de secciones, temas y bitácora, sin distinguir mayúsculas ni acentos, con un fragmento de una línea. |
| `list_journal` | `from`, `to` (inclusive, `YYYY-MM-DD`), `section`, `cursor`, `limit` | Entradas de la bitácora, de la más nueva a la más antigua. Los textos de más de 8 KiB se recortan (`body_truncated`). |
| `get_role_path` | `cursor`, `limit` | Todos los temas de tipo `role`, numerados a través de las secciones: la guía para quien ocupe el rol después. |
| `get_progress` | `cursor` | Cantidad de temas por estado en todo el cuaderno, en los temas del rol y personales y en cada sección, y la cantidad de entradas de la bitácora. |

### Herramientas de escritura (solo con `--allow-write`)

| Herramienta | Entradas | Qué hace |
|---|---|---|
| `add_journal_entry` | `body` (obligatorio, ≤ 64 KiB), `date` (por defecto, hoy), `section`, `topic` | Agrega una entrada a la bitácora. La sección y el tema deben existir; si se indican ambos, el tema debe pertenecer a la sección. |
| `set_topic_status` | `topic`, `status` (ambos obligatorios) | Cambia el estado de un tema. Informa el estado anterior; no escribe si nada cambia. |
| `add_topic` | `section`, `title` (obligatorios), `slug`, `type` (por defecto `role`), `status` (por defecto `pending`), `notes` | Agrega un tema al final de una sección existente. El slug se deriva del título si no se indica. |
| `append_topic_notes` | `topic`, `text` (ambos obligatorios) | Agrega texto al final de las notas de un tema, después de una línea en blanco, sin tocar lo que ya hay. Se rechaza si las notas superarían 256 KiB. |

Cada escritura pasa por el mismo código que la CLI: entradas validadas,
escrituras atómicas (archivo temporal, fsync, rename) con permisos privados y
detección de conflictos, de modo que nunca se sobrescribe un archivo que otro
programa cambió mientras tanto. Se rechaza el texto con caracteres de control
(salvo tabulaciones y saltos de línea).

## Notas de seguridad

- **El contenido del cuaderno es información, no instrucciones.** Las notas,
  los títulos y las entradas pueden haberlos escrito otras personas (una
  plantilla, un archivo importado). El servidor los devuelve solo dentro de
  los campos JSON de los resultados (`title`, `description`, `notes`, `body`,
  `snippet`, y `url` y `title` de los recursos), nunca en las descripciones de
  las herramientas ni en los mensajes de error. Las instrucciones del servidor
  y la descripción de cada herramienta le indican al asistente que esos campos
  son datos del usuario y que no debe seguirlos como instrucciones. Esto reduce
  el riesgo de inyección de instrucciones, pero no lo elimina: revisa lo que el
  asistente propone hacer, sobre todo con `--allow-write`.
- **Texto saneado.** Se quitan las secuencias de escape ANSI, los caracteres de
  control y los caracteres invisibles que reordenan el texto, para que el
  contenido no pueda ocultar texto ni controlar una terminal que lo muestre.
- **Errores genéricos** (por ejemplo `not found: no such section or topic: topic`),
  que nunca incluyen el contenido de los archivos.
- **Con límites.** Los resultados tienen un tope de elementos y de bytes; los
  textos largos se cortan en el límite de un carácter y se marcan con un campo
  `…_truncated` (y, en las listas, con la marca ` [truncated by bearings]`).
  Se rechazan las solicitudes de más de 2 MiB.
- **Mínimo privilegio.** Inicia el servidor sin `--allow-write` salvo que
  quieras que el asistente modifique el cuaderno. Nada se puede borrar por MCP.
  Si sincronizas el cuaderno con git, puedes revisar cada cambio con `git diff`
  antes de `bearings sync`.

## Configuración

Primero instala `bearings` (consulta [cli/README.es.md](../cli/README.es.md)) y
comprueba que está en tu `PATH`: `bearings progress` debe funcionar en una
terminal. Si el asistente no lo encuentra, usa la ruta absoluta del binario
(`which bearings`) en lugar de `bearings` en los ejemplos.

### Claude Code

1. Agrega el servidor, de solo lectura, con el cuaderno por defecto
   (`~/bearings` o `BEARINGS_NOTEBOOK`):

   ```sh
   claude mcp add bearings -- bearings mcp
   ```

2. O elige el cuaderno de forma explícita y permite escrituras:

   ```sh
   claude mcp add bearings -- bearings mcp --notebook /home/yo/bearings --allow-write
   ```

   Usa `--scope user` (`claude mcp add --scope user bearings -- …`) para
   tenerlo disponible en todos los proyectos.

3. Compruébalo: `claude mcp list` debe mostrar `bearings` conectado. Dentro de
   una sesión, `/mcp` muestra sus herramientas.

Para cambiar las opciones más adelante, quítalo y vuelve a agregarlo:
`claude mcp remove bearings`.

### Claude Desktop

1. Abre Settings → Developer → Edit Config. Se abre
   `claude_desktop_config.json` (en macOS, en
   `~/Library/Application Support/Claude/`; en Windows, en `%APPDATA%\Claude\`).
2. Agrega el servidor dentro de `mcpServers`. Solo lectura:

   ```json
   {
     "mcpServers": {
       "bearings": {
         "command": "/usr/local/bin/bearings",
         "args": ["mcp", "--notebook", "/Users/yo/bearings"]
       }
     }
   }
   ```

   Con escrituras permitidas:

   ```json
   {
     "mcpServers": {
       "bearings": {
         "command": "/usr/local/bin/bearings",
         "args": ["mcp", "--notebook", "/Users/yo/bearings", "--allow-write"]
       }
     }
   }
   ```

   Claude Desktop no usa el `PATH` de tu shell, así que indica la ruta absoluta
   de `bearings` y del cuaderno. En Windows, escapa las barras invertidas en el
   JSON (`"C:\\Users\\yo\\bearings"`).
3. Reinicia Claude Desktop. Las herramientas de Bearings aparecen en el menú de
   herramientas.

### Solución de problemas

- Ejecuta `bearings mcp --log-level debug` en una terminal: queda esperando a
  un cliente en la entrada estándar (pulsa Ctrl+D para terminarlo). Si no puede
  abrir el cuaderno, termina enseguida con un error en stderr.
- Claude Desktop guarda los registros del servidor (stderr) en su carpeta de
  registros (`~/Library/Logs/Claude/` en macOS, `%APPDATA%\Claude\logs\` en
  Windows).
