# `bearings sync`

`bearings sync` mantiene un cuaderno en un repositorio git y, si la rama actual
tiene upstream, lo mantiene al día con su remoto.

```
bearings sync              confirma, luego pull --rebase y push
bearings sync --dry-run    muestra qué se confirmaría; no cambia nada
bearings sync --verbose    muestra también qué git se usa
```

Esta guía está en español; la versión en inglés es [sync.md](sync.md).

## Configuración

El directorio del cuaderno debe ser el nivel superior de su propio repositorio
git (no un subdirectorio de otro). Usa un remoto **privado**: el cuaderno guarda
tus notas de trabajo.

```
git -C ~/bearings init
bearings sync                                   # primera confirmación
git -C ~/bearings remote add origin <url de un repositorio privado>
git -C ~/bearings push -u origin HEAD           # define el upstream
```

A partir de ahí, `bearings sync` hace todo. Sin upstream solo confirma, y lo
indica.

## Qué hace

1. Comprueba que `git` (2.26 o posterior) está en el `PATH`, que el cuaderno es
   el nivel superior de un árbol de trabajo, que no hay un rebase ni un merge en
   curso, y que la configuración propia del repositorio es segura (ver más
   abajo).
2. Lista los archivos del cuaderno que cambiaron: rutas `bearings.yaml`,
   `sections/…` y `journal/…` que coinciden exactamente con el diseño del
   cuaderno. Cualquier otra cosa (un `notas.txt` suelto, un archivo de
   intercambio del editor, un archivo de bitácora mal ubicado) nunca se agrega
   y queda sin seguimiento.
3. Agrega al índice y confirma exactamente esas rutas, con el mensaje
   `bearings sync <marca de tiempo UTC>`. Los cambios que hayas preparado tú
   mismo en otros archivos siguen preparados y no se confirman.
4. Con upstream: `git pull --rebase` y luego `git push` al upstream.

`--dry-run` se detiene después del paso 2 y no escribe nada en el repositorio
(ni siquiera en el índice de git). `--json` imprime
`{"dry_run", "changes": [{"path", "change"}], "commit", "upstream", "pulled", "pushed"}`
(`change` es `added`, `modified` o `deleted`).

## Conflictos

Si tus confirmaciones entran en conflicto con cambios del remoto, el rebase se
detiene y `bearings sync` se detiene con él: no se hace push, y el repositorio
queda exactamente como lo dejó git, a medio rebase. Muestra los comandos para
revisar el conflicto (`git status`, `git diff`), para continuar (`git add
<archivo>`, `git rebase --continue`) o para desistir (`git rebase --abort`).
Nunca resuelve conflictos, no hace reset, no guarda cambios en stash y no hace
push forzado. Mientras el rebase no termine, `bearings sync` se niega a
ejecutarse, así que nunca confirma un archivo con marcas de conflicto.

Un push rechazado (alguien subió cambios mientras tanto) conserva tus
confirmaciones localmente; ejecuta `bearings sync` otra vez.

## Seguridad

El directorio del cuaderno puede venir de otra persona, y también su directorio
`.git` (una carpeta copiada, un archivo comprimido descomprimido). La
configuración y los hooks de git dentro de `.git` pueden ejecutar programas, así
que `bearings sync` los trata como hostiles.

**Cómo se ejecuta git.** Siempre mediante `exec` con una lista de argumentos
explícita, nunca a través de una shell, y con `-C <cuaderno>`. Ningún contenido
del cuaderno es nunca un argumento: el mensaje de confirmación es fijo, y las
rutas a confirmar (ya validadas como rutas del diseño del cuaderno) se envían a
git por la entrada estándar (`--pathspec-from-file`). `--literal-pathspecs`
desactiva la coincidencia con comodines. Se eliminan las variables de entorno
que harían que git apuntara a otro repositorio o cambiara la coincidencia de
rutas (`GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_*_PATHSPECS`, …).
`bearings sync` nunca maneja credenciales: los auxiliares de credenciales de
git, el agente SSH y los avisos funcionan como siempre. Sin terminal,
`GIT_TERMINAL_PROMPT=0` evita que git espere una entrada.

**Ajustes que se sobrescriben en cada comando de git** (las opciones `-c` de la
línea de comandos ganan a todos los archivos de configuración):

| Opción | Por qué | Compromiso |
|---|---|---|
| `core.fsmonitor=false` | Un ajuste de fsmonitor es un programa que git ejecuta en `status`, `add` y `commit`. | Ninguno para un repositorio del tamaño de un cuaderno. |
| `core.hooksPath=/dev/null` | Los hooks (`.git/hooks/*`, o un directorio indicado en la configuración) son programas. Una ruta que no puede contener hooks desactiva todos en cada versión de git y en cada plataforma; un valor vacío ha significado cosas distintas en distintas versiones de git. | Tus propios hooks (un escáner de secretos, un formateador) no se ejecutan durante `bearings sync`. Sí se ejecutan cuando usas git directamente. |
| `protocol.ext.allow=never` | El transporte `ext::` ejecuta un comando indicado en la URL del remoto. | Ninguno. |
| `protocol.file.allow=user` | Las rutas locales funcionan como remotos cuando las pides, pero no para los fetch que git inicia por su cuenta (submódulos). | Ninguno. |
| `submodule.recurse=false`, `--no-recurse-submodules` | Los submódulos podrían traer datos de cualquier lugar y ejecutar su propia configuración. | Los submódulos de un repositorio de cuaderno no se actualizan. |
| `push.default=upstream` | Hace push solo de la rama actual, a su upstream. | No se admite un remoto de push distinto (`pushRemote`). |

`GIT_EDITOR` y `GIT_SEQUENCE_EDITOR` se definen como `:` (el editor vacío de
git) y `GIT_PAGER` como `cat`, así que nunca se ejecuta un editor o paginador
configurado.

**Ajustes rechazados cuando el propio repositorio los define.** Algunos ajustes
no pueden sobrescribirse sin sobrescribir también tus valores globales (tu
configuración de clave SSH, tu gestor de contraseñas). En su lugar,
`bearings sync` lee los *nombres* de los ajustes de la configuración propia del
repositorio (`.git/config`, la configuración del árbol de trabajo y todo lo que
estos incluyan; los valores nunca se leen ni se imprimen) y se niega a ejecutarse,
indicando las claves, cuando alguno de estos está definido ahí:

- programas: `core.sshCommand`, `core.askPass`, `core.gitProxy`,
  `core.alternateRefsCommand`, `credential.helper` (también por URL),
  `gpg.program` (y `gpg.<formato>.program`), `gpg.ssh.defaultKeyCommand`,
  `filter.<driver>.clean|smudge|process`, `diff.external`,
  `diff.<driver>.textconv|command`, `merge.<driver>.driver`,
  `remote.<nombre>.uploadpack|receivepack`;
- adónde van los datos y cómo se protegen: `url.<base>.insteadOf` y
  `pushInsteadOf`, `remote.<nombre>.proxy`, `http.proxy`, `http.sslVerify`,
  `http.sslCAInfo`, `http.sslCAPath`, `http.sslCert`, `http.sslKey`,
  `http.cookieFile`, `http.extraHeader`, `http.curloptResolve` (también por
  URL), `protocol.allow` y `protocol.<nombre>.allow`;
- `core.worktree`.

Si tú mismo definiste uno de estos, muévelo a tu configuración global
(`git config --global …`); allí se considera de confianza y sigue funcionando.
Un repositorio que define `core.fsmonitor` o `core.hooksPath` no se rechaza,
porque esos ajustes se sobrescriben; `bearings sync` muestra una advertencia.

Esta lista cubre los ajustes que hacen que git ejecute programas o envíe datos a
otro lugar durante `add`, `commit`, `pull --rebase` y `push`; no puede hacer
confiable a un repositorio que no lo es. Si el cuaderno llegó con el directorio
`.git` de otra persona, lo más seguro es eliminar `.git` y ejecutar `git init`
de nuevo. Revisa a dónde se envía tu cuaderno con `git -C <cuaderno> remote -v`.

Las propias protecciones de git siguen activas: git rechaza los repositorios
que pertenecen a otro usuario (`safe.directory`).
