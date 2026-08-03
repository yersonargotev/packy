# Instalación de packs por proyecto y reproducibilidad para colaboradores

## Pregunta

¿Cómo debe Packy combinar packs globales con recursos locales por proyecto de
modo que un colaborador que clone el repositorio pueda reproducirlos? En
particular, ¿debería `packy pack install matty --surface codex` copiar los
recursos en vez de crear enlaces simbólicos?

## Resumen ejecutivo

La activación local personal, decidida inicialmente, sí es útil como
**consentimiento de un usuario en un checkout**, pero por sí sola no hace el
entorno reproducible para el equipo: ni su estado ni un enlace a una caché del
usuario viajan al clonar. Los tres hosts soportados tienen, en cambio, rutas y
configuración de proyecto que pueden versionarse. Codex descubre skills de
`.agents/skills` en el árbol de trabajo; Claude Code soporta `.claude/skills`,
`.claude/rules` y `.mcp.json`; y OpenCode combina `opencode.json` con
`.opencode/`. [Codex skills](https://learn.chatgpt.com/docs/build-skills),
[Claude Code skills](https://code.claude.com/docs/en/skills),
[Claude Code MCP](https://code.claude.com/docs/en/mcp), [OpenCode config](https://dev.opencode.ai/docs/config#locations),
[OpenCode skills](https://opencode.ai/docs/skills).

La conclusión es separar tres cosas que el verbo actual `activate` mezcla si
se intenta reutilizar para este fin:

1. **Dependencia declarada del proyecto:** qué pack y qué versión exacta espera
   el repositorio. Es versionable y se comparte.
2. **Materialización:** los bytes exactos que se colocan en las rutas nativas
   del host. Para ser portable en Git deben ser una copia (vendored), no un
   symlink hacia el home de quien instaló.
3. **Activación/autoridad personal:** el consentimiento local para efectos que
   no deben venir de un clone, en especial ejecutar hooks, confiar un proyecto,
   autenticar OAuth o proporcionar secretos.

La recomendación incremental es añadir primero un manifiesto versionable de
dependencias de Packs y un lock exacto, más `packy pack install` que resuelva,
copie y verifique únicamente recursos declarativos seguros (skills,
instructions/rules, agents y commands) en las rutas de proyecto nativas. Debe
mostrar un diff y registrar ownership/huellas para desinstalación. MCP,
plugins, hooks, credenciales y confianza del host quedan fuera de la primera
instalación automática; se declaran y se aplican sólo tras consentimiento
personal explícito. No es necesario sustituir `activate`: éste sigue siendo el
flujo global y de autorización de runtime.

Esta es una **recomendación de diseño**, no una afirmación de que los hosts
apliquen una semántica idéntica.

## Hechos de los hosts

La siguiente matriz distingue artefactos que el repositorio puede compartir de
estado que debe seguir siendo personal. “Versionable” significa que la
documentación del host admite el ámbito de proyecto/repository; no implica que
el artefacto sea seguro de ejecutar ni que Packy deba tomar ownership sin una
política adicional.

| Recurso | Codex | Claude Code | OpenCode |
| --- | --- | --- | --- |
| Skills | Descubre `.agents/skills` desde el directorio actual hacia la raíz y también skills globales en `~/.agents/skills`; acepta directorios enlazados. **Versionable:** sí, como árbol de skill dentro del repo. [Fuente](https://learn.chatgpt.com/docs/build-skills) | Las skills de proyecto viven en `.claude/skills`; las personales pueden ser globales y la documentación admite symlinks para skills personales. **Versionable:** sí, mediante el árbol local. [Fuente](https://code.claude.com/docs/en/skills) | Descubre `.opencode/skills` y skills globales. **Versionable:** sí. [Fuente](https://opencode.ai/docs/skills) |
| Instructions / rules | `AGENTS.md` se resuelve jerárquicamente; también puede existir configuración global. **Versionable:** sí, como instrucciones del repo. [Fuente](https://learn.chatgpt.com/docs/agent-configuration/agents-md) | `CLAUDE.md` y `.claude/rules/` admiten instrucciones de proyecto. **Versionable:** sí. [Fuente](https://code.claude.com/docs/en/memory) | Lee `AGENTS.md` y reglas definidas en configuración. **Versionable:** sí, si la regla es deliberada para el repo. [Fuente](https://opencode.ai/docs/rules/) |
| Agents / commands | Codex documenta subagentes configurables; las skills son la superficie portable para workflows reutilizables, no un directorio de slash commands arbitrarios. **Versionable:** depende del artefacto nativo y debe validarse por adaptador. [Fuente](https://learn.chatgpt.com/docs/build-skills) | `.claude/agents/` y comandos/skills de proyecto son superficies nativas. **Versionable:** sí. [Agentes](https://code.claude.com/docs/en/sub-agents) | `.opencode/agents/` y `.opencode/commands/` son superficies de proyecto. **Versionable:** sí. [Agentes](https://opencode.ai/docs/agents/), [commands](https://opencode.ai/docs/commands/) |
| MCP / plugins | La configuración de proyecto reside en `.codex/config.toml` y requiere proyecto confiable; MCP se configura en el host. **No versionar secretos ni tratar la confianza como compartida.** [Config](https://learn.chatgpt.com/docs/config-file/config-advanced), [MCP](https://learn.chatgpt.com/docs/extend/mcp) | `.mcp.json` puede compartirse; configuración local/usuario puede quedar en `~/.claude.json`. Plugins y marketplaces tienen caché personal. [MCP](https://code.claude.com/docs/en/mcp), [plugins](https://code.claude.com/docs/en/discover-plugins), [caché](https://code.claude.com/docs/en/plugins-reference#plugin-caching-and-file-resolution) | `opencode.json` de proyecto puede definir MCP/plugins; OAuth y sus tokens continúan siendo personales. [MCP](https://opencode.ai/docs/mcp-servers/), [plugins](https://opencode.ai/docs/plugins/), [config](https://dev.opencode.ai/docs/config#locations) |

Dos límites prácticos se desprenden de esos hechos:

- Copiar una skill completa —incluidos `references/`, scripts y assets— es más
  fiel que copiar sólo `SKILL.md`; los formatos de skills admiten esos archivos
  auxiliares. [Codex](https://learn.chatgpt.com/docs/build-skills),
  [Claude Code](https://code.claude.com/docs/en/skills),
  [OpenCode](https://opencode.ai/docs/skills).
- Un archivo de configuración compartible puede contener una definición MCP,
  pero no concede confianza del proyecto, autentica cuentas ni debe transportar
  secretos. Esto es una inferencia de las separaciones documentadas entre
  configuración de proyecto, configuración de usuario y OAuth/trust de los
  hosts, por lo que Packy debe solicitar esas acciones a cada usuario.

## Patrones comparables

Vercel `npx skills` instala por defecto en el proyecto. Su documentación
recomienda enlaces simbólicos a una copia canónica para recibir actualizaciones
de esa copia y ofrece `--copy` cuando se necesita materializar los archivos;
no documenta un lockfile de dependencias. [Vercel Skills: scopes de
instalación](https://github.com/vercel-labs/skills#installation-scope).

GitHub `gh skill install` también usa por defecto el ámbito de proyecto, copia
los ficheros, conserva metadatos de origen para tracking y permite fijar un tag
o SHA; no documenta un lockfile general. [Manual de `gh skill
install`](https://cli.github.com/manual/gh_skill_install).

Estos patrones validan dos necesidades distintas, no una única solución:

- el symlink es cómodo para un desarrollador que desea actualizar desde una
  fuente canónica local;
- la copia con origen/versiones fijados permite que el árbol sea visible al
  colaborador y revisable por Git.

Para Packy, el segundo es el requisito relevante cuando se promete
reproducibilidad de equipo. Un symlink dentro del repo hacia
`~/.packy/...` rompe al clonar; un symlink relativo hacia un árbol que sí está
commiteado no aporta una ventaja importante frente a usar el árbol directamente
y complica ownership y herramientas que no lo sigan.

## Contraste con Packy actual

Actualmente Packy es un configurador de workstation: persiste activación,
contribuyentes, recovery y ownership en `~/.packy/packs.json`, vincula skills
globales a su Installed Source e inyecta/incluye instrucciones globales por
superficie. [README](../../README.md#L98-L110), [Claude global layout](../claude-code.md#L15-L30),
[ADR 0006](../adr/0006-own-workstation-layout-by-domain.md).

El código de los adaptadores Codex, OpenCode y Claude confirma que las
proyecciones de skill actuales se planifican como enlaces simbólicos y que las
instrucciones se controlan como archivos o contribuciones globales. Esto es
evidencia interna, no una limitación de los hosts: [Codex adapter](../../internal/codex/surface.go),
[OpenCode adapter](../../internal/opencode/surface.go),
[Claude adapter](../../internal/claudecode/surface.go).

Por tanto, añadir sólo un selector `--scope project` al estado actual produciría
un consentimiento por checkout pero **no** una instalación compartible. Y
cambiar globalmente de links a copias empeoraría las actualizaciones globales
sin resolver la declaración/versionado del proyecto.

## Alternativas

| Alternativa | Colaborador al clonar | Actualización / desinstalación | Evaluación |
| --- | --- | --- | --- |
| Activación personal por checkout, con symlinks a Packy Home | No recibe recursos ni estado; debe activar manualmente | Sencilla para un usuario; ownership permanece en `~/.packy` | Mantener para consentimiento local, pero insuficiente para reproducibilidad. |
| Vendoring/copia de recursos host-nativos | Recibe los bytes commiteados | Git muestra el diff; Packy necesita huellas/origen para actualizar o borrar sólo lo suyo | Menor incremento que satisface el objetivo de equipo. Puede crecer el repo. |
| Manifiesto + lock + caché + materialización | Puede ejecutar `packy install` y obtener exactamente lo fijado, aun si no se commitean bytes | Buenas actualizaciones/dedupe; exige resolución, caché, lock y política de drift | Arquitectura destino razonable, pero no requerir caché compartida ni plugins en la primera entrega. |
| Plugins nativos de cada host | Aprovecha instalación/caché del host | Semántica, trust y distribución distintas por host | Útil sólo cuando un pack sea realmente un plugin del host; no sustituye recursos portables ni un manifiesto común. |

## Recomendación incremental

Esta sección conserva la progresión de la recomendación de investigación. Las
decisiones posteriores fijaron nombres, versiones exactas y la closure completa;
ADR 0027 es la autoridad para esos puntos.

### Primer incremento útil

1. Introducir `packy.json` como declaración de proyecto con dependencias
   explícitas (pack, versión exacta, recursos seleccionados y superficies) y
   `packy.lock.json` con la identidad inmutable ya resuelta por Packy: versión
   del Pack, fuente, artefacto y digests de cada árbol. Ambos formatos tienen
   esquemas versionados y no reutilizan `packs.json`, que es estado personal de
   ownership y recuperación.
2. Implementar `packy pack install <pack> --surface <host>` como operación
   desde la raíz Git. Debe resolver/refrescar el lock con consentimiento claro,
   previsualizar el diff y copiar una **closure completa, regular y
   verificada** a la ruta local nativa del host. La copia pasa a formar parte
   del repo y puede revisarse/commitearse.
3. Limitar esa primera operación a skills, instructions/rules, agents y
   commands que el adaptador declare seguros y representables. Para Codex, un
   “command” debe seguir la degradación ya declarada por Packy a workflow-skill
   cuando no exista equivalente nativo; no simular slash commands. [Mapeo
   Addy](addy-capability-mapping.md#L59-L66).
4. Registrar por proyecto y por recurso su fingerprint, origen y contribuidor.
   `packy pack update` compara el lock, el árbol materializado y las huellas;
   `deactivate`/`uninstall` sólo borra lo que siga siendo exactamente propiedad
   de Packy y preserva drift o contenido ajeno.

El comando propuesto es razonable, con una precisión: `install` debe significar
“declarar/fijar y materializar recursos reproducibles”, no “activar cualquier
capacidad”. Una interfaz inicial coherente puede ser:

```sh
packy pack install matty --surface codex
# escribe declaración + lock; previsualiza y copia recursos declarativos

packy pack activate matty --surface codex --project
# obtiene por separado el consentimiento/runtime personal del proyecto
```

Si posteriormente se desea activar los recursos materializados para el usuario
actual, debe ser una opción explícita y posterior, no un efecto implícito del
clone ni del `install`.

### Fuera del primer incremento

La investigación recomendó inicialmente posponer MCP, plugins y hooks para
reducir el primer incremento. La decisión de producto posterior amplió la misma
fase para cubrir la closure completa: la instalación materializa definiciones
MCP no secretas y artefactos de hooks, mientras una activación de proyecto
separada conserva el consentimiento personal antes de habilitar efectos de
runtime. Credenciales, OAuth y trust permanecen fuera de Git. Los plugins
nativos siguen necesitando un adaptador y evidencia propios, no una copia
genérica de archivos.

## Seguridad, ownership, actualización y drift

- **Seguridad:** copiar hace visible y revisable el código/script de una skill,
  pero no lo vuelve inocuo. La instalación debe conservar la admisión de
  procedencia, licencia, digest y preview de Packy; cualquier ejecución sigue
  bajo los permisos del host. Los secretos no pertenecen al lock ni al árbol
  vendored.
- **Ownership:** la marca debe estar ligada a `(raíz Git, pack, recurso,
  superficie, lock digest y destino)`, no sólo a una ruta. Así dos packs no
  pueden adoptar por parecido una copia existente.
- **Update:** cambiar una versión requiere resolver de nuevo, actualizar el
  lock y presentar el diff completo. Un update no debe seguir una rama mutable
  ni modificar silenciosamente recursos editados por el usuario.
- **Deactivate/uninstall:** debe retirar sólo el último contribuidor y sólo si
  el contenido aún coincide con el recibo/huella. Ante un archivo modificado,
  ambiguo o ajeno, preservarlo e informar drift.
- **Drift:** distinguir “el repo difiere del lock” de “la caché personal no
  contiene el artefacto”. El primero requiere decisión del equipo; el segundo
  se puede reparar reacquiriendo el artefacto exacto fijado.

Estas reglas prolongan el modelo existente de ownership y reversión
receipted, en vez de inferir propiedad por una ruta o bytes parecidos. [ADR
0024](../adr/0024-reverse-only-receipted-external-configuration.md).

## Efecto sobre las decisiones ya aceptadas

Siguen vigentes:

- **ADR 0026:** global y proyecto se componen aditivamente. Una instalación de
  proyecto agrega recursos declarados; no es una máscara de la activación
  global.
- **Project root:** la raíz del worktree Git sigue siendo el identificador
  correcto para la declaración, lock y ownership local.
- **Activación local personal:** sigue siendo correcta para consentimiento,
  trust, OAuth, estado de runtime y recuperación personal.
- **Default global de lifecycle:** debe conservarse para no cambiar el
  significado de los comandos existentes por el directorio actual.

Debe reconsiderarse una sola formulación: “la activación de proyecto no se
comparte automáticamente mediante el repositorio” no puede ser la historia
completa del producto. La **activación** no se comparte; la nueva
**declaración/lock y la copia materializada** sí pueden versionarse y viajar
con el repositorio. Esta separación satisface tanto el consentimiento
individual como la reproducibilidad del equipo.

## Fuentes primarias

- [OpenAI: skills](https://learn.chatgpt.com/docs/build-skills),
  [AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md),
  [configuración avanzada](https://learn.chatgpt.com/docs/config-file/config-advanced),
  [MCP](https://learn.chatgpt.com/docs/extend/mcp), y [plugins](https://developers.openai.com/plugins/build/plugins).
- [Anthropic: memory/instructions](https://code.claude.com/docs/en/memory),
  [skills](https://code.claude.com/docs/en/skills), [subagents](https://code.claude.com/docs/en/sub-agents),
  [settings](https://code.claude.com/docs/en/settings), [MCP](https://code.claude.com/docs/en/mcp),
  [plugins](https://code.claude.com/docs/en/discover-plugins), y [referencia de plugins](https://code.claude.com/docs/en/plugins-reference).
- [OpenCode: config](https://dev.opencode.ai/docs/config#locations),
  [rules](https://opencode.ai/docs/rules/), [skills](https://opencode.ai/docs/skills),
  [agents](https://opencode.ai/docs/agents/), [commands](https://opencode.ai/docs/commands),
  [MCP](https://opencode.ai/docs/mcp-servers/) y [plugins](https://opencode.ai/docs/plugins/).
- [Vercel Skills](https://github.com/vercel-labs/skills#installation-scope) y
  [GitHub CLI `gh skill install`](https://cli.github.com/manual/gh_skill_install).
