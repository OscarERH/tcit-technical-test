# Challenge TCIT: Posts

Aplicación para crear, listar, filtrar y eliminar posts. Cada post tiene un nombre y una descripción.

## Cómo ejecutarla

Solo se necesita Docker.

```bash
docker compose up --build
```

- Aplicación: http://localhost:4200
- API: http://localhost:8080/api/posts

Si esos puertos están ocupados, cámbialos con `WEB_PORT` y `API_PORT` en un archivo `.env`
(hay un ejemplo en `.env.example`).

El emulador de Cloud Spanner guarda los datos solo en memoria. Si se reinicia el contenedor
`spanner`, los posts se pierden y la API vuelve a crear el esquema al arrancar.

## Stack

| Requerido | Implementación en este repositorio |
| --- | --- |
| Angular + TypeScript + Google Material | `frontend/`: Angular 22 (componentes standalone, signals) con Angular Material 3 |
| Go sobre Borg | `backend/`: API JSON en Go 1.26 (`net/http`), empaquetada como contenedor sin estado |
| OneStack Workflows con Symfony | `workflow/`: servicio Symfony 8 con el componente Workflow |
| Cloud Spanner | Emulador oficial de Cloud Spanner; el mismo código funciona contra Spanner real |
| Google Drive API | Adjuntos de un post subidos a Drive (opcional, requiere credenciales) |
| Gemini API | Completar el formulario desde un brief de campaña (opcional, requiere API key) |

Borg y OneStack son plataformas internas de Google y no se pueden ejecutar fuera de ella.
Aquí cada servicio es un contenedor sin estado que se configura con variables de entorno, y
Docker Compose hace de orquestador. Un job de Borg se empaqueta de la misma manera.

```
navegador ──> web (nginx + Angular) ──/api──> api (Go) ──> spanner (emulador)
                                                 ├──> workflow (Symfony)
                                                 ├──> Gemini API      (opcional)
                                                 └──> Google Drive    (opcional)
```

## Requisitos del challenge

| Requisito | Cómo se cumple |
| --- | --- |
| Insertar, eliminar y listar posts | `POST /api/posts` retorna el post creado, `DELETE /api/posts/{id}` retorna el eliminado y `GET /api/posts` retorna todos |
| Filtrar por nombre localmente | El filtro recorre los posts que ya están en memoria (`PostsStore.filteredPosts`) y no llama a la API |
| Pedir la lista completa una sola vez por carga | `PostsStore.load()` ignora las llamadas posteriores. Crear, eliminar o cambiar de estado actualiza la lista local con la respuesta de la API |
| Formulario, filtro y lista | Componentes `PostForm`, `PostFilter` y `PostList` |
| Convenciones de nombres | JSON y TypeScript en camelCase, tablas y columnas de Spanner en snake_case |

La vista se aparta de la propuesta visual en un punto: el formulario va en una tarjeta
propia en vez de una fila bajo la tabla. En pantallas anchas queda al lado de la lista y en
pantallas angostas, debajo.

## API

| Método | Ruta | Descripción |
| --- | --- | --- |
| GET | `/api/posts` | Lista todos los posts |
| POST | `/api/posts` | Crea un post `{ name, description }` y lo retorna |
| DELETE | `/api/posts/{id}` | Elimina un post y lo retorna |
| POST | `/api/posts/{id}/transitions` | Aplica una transición del workflow `{ transition }` |
| POST | `/api/posts/{id}/attachments` | Sube un adjunto a Drive (multipart, campo `file`) |
| POST | `/api/briefs/extract` | Extrae `{ name, description }` de un brief `{ brief }` |
| GET | `/api/config` | Integraciones disponibles y transiciones del workflow |

Los errores tienen la forma `{ "error": { "message": "...", "details": { ... } } }`.

## Integraciones

### Workflow (Symfony)

Un post pasa por tres estados: `draft → published → archived → draft`. Las reglas están en
`workflow/config/packages/workflow.yaml` y en ningún otro lugar. La API en Go le pregunta al
servicio Symfony antes de cambiar el estado de un post, y la interfaz arma los botones
(Publicar, Archivar, Restaurar) con esa misma definición. Si el servicio no responde, el
CRUD sigue funcionando y la columna de estado no se muestra.

### Gemini

Con `GEMINI_API_KEY` en `.env` aparece el botón "Completar con IA". El usuario pega el brief
de una campaña, Gemini propone un nombre y una descripción, y el usuario los revisa antes de
crear el post. El modelo se elige con `GEMINI_MODEL`.

El brief puede traer información confidencial, así que se trata con cuidado:

- La API key vive solo en el backend y viaja en un header, nunca en la URL ni al navegador.
- El brief no se guarda en la base de datos ni se escribe en los logs, tampoco cuando
  Gemini responde con error.
- A Gemini se envía solo el texto del brief, sin datos del usuario.
- El prompt le indica a Gemini que ignore las instrucciones que vengan dentro del brief, y
  la respuesta se limita a un esquema JSON con dos campos.

El nivel gratuito de AI Studio permite a Google usar los datos enviados para mejorar sus
productos. Donde haya exigencias de cumplimiento, la API key debe ser de un proyecto con
facturación, o se puede apuntar a Vertex AI con `GEMINI_BASE_URL`.

### Google Drive

Para habilitar los adjuntos:

1. Crear una cuenta de servicio con la Drive API habilitada y guardar su llave en
   `secrets/drive-service-account.json` (la carpeta está en `.gitignore`).
2. Agregar la cuenta de servicio como miembro de una unidad compartida y copiar el ID
   de una carpeta dentro de ella. Una carpeta de "Mi unidad" no sirve, porque las cuentas
   de servicio no tienen cuota de almacenamiento propia.
3. En `.env`: `DRIVE_CREDENTIALS_FILE=/secrets/drive-service-account.json` y
   `DRIVE_FOLDER_ID=<id de la carpeta>`.

El archivo se sube a Drive y Spanner guarda solo la referencia, en la tabla
`post_attachments` (intercalada en `posts`). Al eliminar un post también se eliminan sus
archivos de Drive.

## Pruebas

Las pruebas corren dentro de Docker, así que no hace falta instalar Go, Node ni PHP.

```bash
make test            # todo
make test-backend    # Go
make test-frontend   # Angular
make test-workflow   # Symfony
```

| Parte | Qué se prueba |
| --- | --- |
| Backend (`go test`) | Handlers HTTP con dobles en memoria: CRUD, validaciones, transiciones, adjuntos y extracción de briefs. Clientes de Gemini y del workflow contra servidores HTTP falsos, incluidos sus errores. Repositorio contra el emulador de Cloud Spanner, con una base de datos nueva por prueba. |
| Frontend (Vitest) | `PostsStore`: la lista se pide una sola vez, el filtro es local y por nombre, y crear, eliminar y cambiar de estado actualizan la lista sin recargarla. Vista completa: filtro, lista y formulario con componentes reales y solo HTTP simulado. |
| Workflow (PHPUnit) | Definición del ciclo de vida, transiciones permitidas y prohibidas, y entradas inválidas. |

Las pruebas del repositorio necesitan el emulador de Spanner. `make test-backend` lo levanta
y define `SPANNER_EMULATOR_HOST`; sin esa variable, `go test` las omite.

Ninguna prueba automática llama a Gemini ni a Google Drive, porque ambos requieren
credenciales. La extracción con Gemini la probé a mano con una API key real. La subida a
Drive todavía no la he probado contra el servicio.

## Estructura

```
backend/    API en Go
  cmd/api/            arranque y configuración
  internal/posts/     dominio, handlers HTTP e interfaces
  internal/store/     repositorio y esquema en Cloud Spanner
  internal/gemini/    cliente de Gemini
  internal/drive/     cliente de Google Drive
  internal/workflow/  cliente del servicio Symfony
frontend/   Angular + Material
  src/app/posts/      store, cliente HTTP y componentes
workflow/   Symfony Workflow
```
