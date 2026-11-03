# Bearings

> Orientate en un rol nuevo.

Bearings te ayuda a **registrar** lo que aprendés en un trabajo nuevo, **seguir**
lo que te falta aprender y **consultarlo** después. Todo lo que es parte del rol
(no tus notas personales) forma una **ruta del rol**: una guía clara y ordenada
para quien ocupe el puesto después de vos.

[Read in English](README.md)

## Partes

| Parte | Qué es | Estado |
|---|---|---|
| [`cli/`](cli/) | Herramienta de línea de comandos `bearings` (Go). Tu cuaderno es una carpeta de archivos Markdown, sincronizada con git. | En desarrollo |
| [`cli/internal/mcpserver`](docs/mcp.es.md) | Servidor MCP local (`bearings mcp`) para que asistentes como Claude lean y actualicen tu cuaderno. | En desarrollo |
| [`web/`](web/) | App web instalable (funciona sin conexión, en compu y celular). Los datos quedan en tu navegador; importás y exportás cuadernos como archivos. | En desarrollo |
| [`format/`](format/SPEC.md) | El formato de archivos del cuaderno, común a todas las partes. | v1 |
| [`templates/`](templates/) | Cuadernos iniciales para roles específicos. Se aceptan contribuciones. | |

## Privacidad

Tus notas no salen de tus dispositivos salvo que vos las muevas. Sin cuentas,
sin backend, sin telemetría. Ver [SECURITY.md](SECURITY.md).

## Licencia

[MIT](LICENSE)
