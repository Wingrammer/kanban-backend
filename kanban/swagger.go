package kanban

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func registerSwaggerRoutes(router *gin.Engine) {
	router.GET("/swagger", redirectToSwagger)
	router.GET("/swagger/", redirectToSwagger)
	router.GET("/swagger/index.html", swaggerUI)
	router.GET("/swagger/openapi.json", openAPISpec)
}

func redirectToSwagger(ctx *gin.Context) {
	ctx.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
}

func swaggerUI(ctx *gin.Context) {
	ctx.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Kanban API - Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: "/swagger/openapi.json",
        dom_id: "#swagger-ui",
        persistAuthorization: true,
        tryItOutEnabled: true
      });
    };
  </script>
</body>
</html>`))
}

func openAPISpec(ctx *gin.Context) {
	spec := strings.ReplaceAll(openAPISpecJSON, "__BEARER__", bearerScheme)
	spec = strings.ReplaceAll(spec, "****** user access token>", bearerScheme+"<Supabase user access token>")
	ctx.Data(http.StatusOK, "application/json; charset=utf-8", []byte(spec))
}

const openAPISpecJSON = `{
  "openapi": "3.0.3",
  "info": {
    "title": "Kanban API",
    "description": "Task API backed by Supabase Postgres with row-level security.",
    "version": "1.0.0"
  },
  "servers": [
    {
      "url": "/"
    }
  ],
  "components": {
    "securitySchemes": {
      "supabaseAccessToken": {
        "type": "apiKey",
        "in": "header",
        "name": "Authorization",
        "description": "Enter: Bearer <Supabase user access token>"
      }
    },
    "schemas": {
      "Task": {
        "type": "object",
        "required": [
          "id",
          "title",
          "status",
          "user_id"
        ],
        "properties": {
          "id": {
            "type": "string"
          },
          "title": {
            "type": "string"
          },
          "status": {
            "type": "string"
          },
          "user_id": {
            "type": "string"
          },
          "created_at": {
            "type": "string",
            "format": "date-time"
          },
          "description": {
            "type": "string"
          },
          "priority": {
            "type": "string"
          },
          "due_date": {
            "type": "string",
            "format": "date-time"
          },
          "assignee_id": {
            "type": "string"
          }
        }
      }
    }
  },
  "security": [
    {
      "supabaseAccessToken": []
    }
  ],
  "paths": {
    "/auth/guest": {
      "post": {
        "summary": "Create an anonymous guest session",
        "description": "Returns a Supabase access token for a new anonymous user. Anonymous sign-ins must be enabled in Supabase Auth.",
        "security": [],
        "responses": {
          "200": {
            "description": "Guest session"
          },
          "502": {
            "description": "Supabase Auth error"
          }
        }
      }
    },
    "/auth/link": {
      "post": {
        "summary": "Link the current guest to a permanent account",
        "description": "Links the bearer-authenticated anonymous user to an email and password.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "email",
                  "password"
                ],
                "properties": {
                  "email": {
                    "type": "string",
                    "format": "email"
                  },
                  "password": {
                    "type": "string",
                    "format": "password",
                    "minLength": 6
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Linked user"
          },
          "400": {
            "description": "Invalid request"
          },
          "401": {
            "description": "Bearer token required"
          }
        }
      }
    },
    "/users/{user_id}/tasks": {
      "get": {
        "summary": "List tasks for a user",
        "parameters": [
          {
            "name": "user_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Tasks",
            "content": {
              "application/json": {
                "schema": {
                  "type": "array",
                  "items": {
                    "$ref": "#/components/schemas/Task"
                  }
                }
              }
            }
          },
          "401": {
            "description": "Bearer token required"
          }
        }
      }
    },
    "/tasks/{task_id}": {
      "get": {
        "summary": "Get a task",
        "parameters": [
          {
            "name": "task_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Task",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Task"
                }
              }
            }
          },
          "404": {
            "description": "Task not found"
          }
        }
      },
      "patch": {
        "summary": "Update fields on a task owned by the bearer-authenticated user",
        "parameters": [
          {
            "name": "task_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "additionalProperties": false,
                "minProperties": 1,
                "properties": {
                  "title": {
                    "type": "string",
                    "minLength": 1,
                    "maxLength": 160
                  },
                  "description": {
                    "type": "string",
                    "maxLength": 4000
                  },
                  "status": {
                    "type": "string",
                    "enum": [
                      "pending",
                      "in_progress",
                      "review",
                      "completed"
                    ]
                  },
                  "priority": {
                    "type": "string",
                    "enum": [
                      "low",
                      "medium",
                      "high"
                    ]
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Updated task",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Task"
                }
              }
            }
          },
          "400": {
            "description": "Invalid changes or unknown fields"
          },
          "401": {
            "description": "Bearer token required"
          },
          "404": {
            "description": "Task not found or not owned by this user"
          },
          "502": {
            "description": "Database unavailable"
          }
        }
      }
    },
    "/tasks": {
      "post": {
        "summary": "Create a task",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/Task"
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Created task",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Task"
                }
              }
            }
          },
          "400": {
            "description": "Invalid JSON"
          }
        }
      }
    },
    "/auth/refresh": {
      "post": {
        "summary": "Refresh an existing Supabase session",
        "security": [],
        "description": "Called by the trusted Next.js server. The refresh token is never exposed to browser JavaScript.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "additionalProperties": false,
                "required": [
                  "refresh_token"
                ],
                "properties": {
                  "refresh_token": {
                    "type": "string",
                    "minLength": 1
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Refreshed session (no-store)"
          },
          "400": {
            "description": "Invalid request"
          },
          "401": {
            "description": "Invalid refresh token"
          },
          "502": {
            "description": "Authentication provider unavailable"
          }
        }
      }
    }
  }
}`
