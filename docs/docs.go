// Package docs Code generated by swaggo/swag. DO NOT EDIT
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "termsOfService": "http://swagger.io/terms/",
        "contact": {
            "name": "Norlis Viamonte",
            "url": "http://www.example.com/support",
            "email": "norlis.viamonte@gmail.com"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/api/auth/token": {
            "post": {
                "description": "authentication token",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "auth"
                ],
                "summary": "Token",
                "parameters": [
                    {
                        "description": "Payload",
                        "name": "data",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/auth.TokenRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Token generated successfully",
                        "schema": {
                            "type": "object",
                            "properties": {
                                "token": {
                                    "type": "string"
                                }
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            }
        },
        "/api/box": {
            "get": {
                "description": "all templates",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "templates"
                ],
                "summary": "List templates",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/models.Box"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            },
            "post": {
                "description": "insert or update templates on s3",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "templates"
                ],
                "summary": "Upsert templates",
                "parameters": [
                    {
                        "description": "Upsert template",
                        "name": "data",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/handlers.CommandBox"
                        }
                    },
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            }
        },
        "/api/box/{service}/{stage}/{template}": {
            "get": {
                "description": "detail",
                "produces": [
                    "text/plain"
                ],
                "tags": [
                    "templates"
                ],
                "summary": "Retrieve template",
                "parameters": [
                    {
                        "type": "string",
                        "description": "service name",
                        "name": "service",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "stage",
                        "name": "stage",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "template name",
                        "name": "template",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            },
            "head": {
                "description": "Check the existence of the template",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "templates"
                ],
                "summary": "Exist template",
                "parameters": [
                    {
                        "type": "string",
                        "description": "service name",
                        "name": "service",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "stage",
                        "name": "stage",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "template name",
                        "name": "template",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "object",
                            "properties": {
                                "exit": {
                                    "type": "boolean"
                                }
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            }
        },
        "/api/box/{service}/{stage}/{template}/build": {
            "get": {
                "description": "replace vars patterns",
                "produces": [
                    "text/plain"
                ],
                "tags": [
                    "templates"
                ],
                "summary": "Build template",
                "parameters": [
                    {
                        "type": "string",
                        "description": "service name",
                        "name": "service",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "stage",
                        "name": "stage",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "template name",
                        "name": "template",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            }
        },
        "/api/box/{service}/{stage}/{template}/vars": {
            "get": {
                "description": "show all vars in template",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "templates"
                ],
                "summary": "List vars template",
                "parameters": [
                    {
                        "type": "string",
                        "description": "service name",
                        "name": "service",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "stage",
                        "name": "stage",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "template name",
                        "name": "template",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "type": "string"
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            }
        },
        "/api/entry": {
            "post": {
                "description": "insert / update vars",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "entry"
                ],
                "summary": "Upsert entries",
                "parameters": [
                    {
                        "description": "Upsert template",
                        "name": "data",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/models.Entry"
                            }
                        }
                    },
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            }
        },
        "/api/entry/key": {
            "get": {
                "description": "detail",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "entry"
                ],
                "summary": "Retrieve key",
                "parameters": [
                    {
                        "type": "string",
                        "description": "key path",
                        "name": "v",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/models.Entry"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            },
            "delete": {
                "description": "delete keys \u0026 children",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "entry"
                ],
                "summary": "Delete",
                "parameters": [
                    {
                        "type": "string",
                        "description": "key path",
                        "name": "v",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "object",
                            "properties": {
                                "message": {
                                    "type": "string"
                                }
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            }
        },
        "/api/entry/prefix": {
            "get": {
                "description": "list all keys by path",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "entry"
                ],
                "summary": "Filter by prefix",
                "parameters": [
                    {
                        "type": "string",
                        "description": "key path",
                        "name": "v",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/models.Entry"
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            }
        },
        "/api/track/key": {
            "get": {
                "description": "history changes",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "entry"
                ],
                "summary": "History",
                "parameters": [
                    {
                        "type": "string",
                        "description": "key path",
                        "name": "v",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Bearer | Basic",
                        "name": "authorization",
                        "in": "header",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/models.Tracking"
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    },
                    "500": {
                        "description": "Internal error",
                        "schema": {
                            "$ref": "#/definitions/problem.ProblemDetail"
                        }
                    }
                }
            }
        },
        "/health": {
            "get": {
                "description": "status format json",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json",
                    "application/json"
                ],
                "tags": [
                    "status"
                ],
                "summary": "health",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/health.Health"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "auth.TokenRequest": {
            "type": "object",
            "properties": {
                "password": {
                    "type": "string"
                },
                "username": {
                    "type": "string"
                }
            }
        },
        "handlers.CommandBox": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "string",
                    "example": "123"
                },
                "payload": {
                    "$ref": "#/definitions/models.Box"
                }
            }
        },
        "health.Health": {
            "type": "object",
            "properties": {
                "hostname": {
                    "type": "string"
                },
                "service": {
                    "type": "string"
                },
                "startedAt": {
                    "type": "string"
                }
            }
        },
        "models.Box": {
            "type": "object",
            "properties": {
                "service": {
                    "type": "string"
                },
                "stage": {
                    "type": "object",
                    "additionalProperties": {
                        "$ref": "#/definitions/models.Stage"
                    }
                }
            }
        },
        "models.Entry": {
            "type": "object",
            "properties": {
                "key": {
                    "type": "string",
                    "example": "development/service/var-example"
                },
                "path": {
                    "type": "string"
                },
                "secure": {
                    "type": "boolean",
                    "example": false
                },
                "value": {
                    "type": "string",
                    "example": "value 123"
                }
            }
        },
        "models.Stage": {
            "type": "object",
            "properties": {
                "template": {
                    "$ref": "#/definitions/models.Template"
                }
            }
        },
        "models.Template": {
            "type": "object",
            "properties": {
                "name": {
                    "description": "s3 path",
                    "type": "string"
                },
                "value": {
                    "type": "string"
                }
            }
        },
        "models.Tracking": {
            "type": "object",
            "properties": {
                "key": {
                    "type": "string"
                },
                "secure": {
                    "type": "boolean"
                },
                "updatedAt": {
                    "type": "string"
                },
                "updatedBy": {
                    "type": "string"
                },
                "value": {
                    "type": "string"
                }
            }
        },
        "problem.ProblemDetail": {
            "type": "object",
            "properties": {
                "detail": {
                    "type": "string",
                    "example": "invalid credentials"
                },
                "instance": {
                    "type": "string",
                    "example": "/api/example"
                },
                "requestId": {
                    "type": "string",
                    "example": "123"
                },
                "stackTrace": {
                    "type": "string"
                },
                "status": {
                    "type": "integer",
                    "example": 401
                },
                "timestamp": {
                    "type": "string",
                    "example": "2024-12-11T20:23:55.248212-03:00"
                },
                "title": {
                    "type": "string",
                    "example": "Unauthorized"
                },
                "type": {
                    "type": "string",
                    "example": "Err"
                }
            }
        }
    },
    "securityDefinitions": {
        "BasicAuth": {
            "type": "basic"
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0",
	Host:             "",
	BasePath:         "/",
	Schemes:          []string{},
	Title:            "nbox API",
	Description:      "Esta es una API generada automáticamente con Swaggo.",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}