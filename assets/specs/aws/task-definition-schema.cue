package aws

import "strings"

#Meta: {
	id:         "aws:ecs:task-definition"
	name:       "AWS ECS Task Definition"
	version:    "1.0"
  matchPatterns: ["taskdef*.json", "*-task.json", "task-definition*.json"]
}

#Schema: {
	@jsonschema(schema="http://json-schema.org/draft-07/schema#")

	close({
		family!: string

		taskRoleArn?: string

		executionRoleArn?: string

		networkMode?: "bridge" | "host" | "awsvpc" | "none"

		containerDefinitions!: [...close({
			name!: string

			image!: string

			repositoryCredentials?: close({
				credentialsParameter!: string
			})

			cpu?: int

			memory?: int

			memoryReservation?: int

			links?: [...string]

			portMappings?: [...close({
				containerPort?: int

				hostPort?: int

				protocol?: "tcp" | "udp"

				appProtocol?: "http" | "http2" | "grpc"

				containerPortRange?: string

				name?: string
			})]

			essential?: bool

			entryPoint?: [...string]

			command?: [...string]

			environment?: [...close({
				name!: string & =~"^[A-Z0-9_]+$"

				value?: string
			})]

			environmentFiles?: [...close({
				value!: string

				type!: "s3"
			})]

			mountPoints?: [...close({
				sourceVolume?: string

				containerPath?: string

				readOnly?: bool
			})]

			volumesFrom?: [...close({
				sourceContainer?: string

				readOnly?: bool
			})]

			linuxParameters?: close({
				capabilities?: close({
					add?: [...string]

					drop?: [...string]
				})

				devices?: [...close({
					hostPath!: string

					containerPath?: string

					permissions?: [..."read" | "write" | "mknod"]
				})]

				initProcessEnabled?: bool

				sharedMemorySize?: int

				tmpfs?: [...close({
					containerPath!: string

					size!: int

					mountOptions?: [...string]
				})]

				maxSwap?: int

				swappiness?: int
			})

			secrets?: [...close({
				name!: string & =~"^[A-Z0-9_]+$"

				valueFrom!: string
			})]

			dependsOn?: [...close({
				containerName!: string

				condition!: "START" | "COMPLETE" | "SUCCESS" | "HEALTHY"
			})]

			startTimeout?: int

			stopTimeout?: int

			hostname?: string

			user?: string

			workingDirectory?: string

			disableNetworking?: bool

			privileged?: bool

			readonlyRootFilesystem?: bool

			dnsServers?: [...string]

			dnsSearchDomains?: [...string]

			extraHosts?: [...close({
				hostname!: string

				ipAddress!: string
			})]

			dockerSecurityOptions?: [...string]

			interactive?: bool

			pseudoTerminal?: bool

			dockerLabels?: {
				"insert-key"?: string
				...
			}

			ulimits?: [...close({
				name!: "core" | "cpu" | "data" | "fsize" | "locks" | "memlock" | "msgqueue" | "nice" | "nofile" | "nproc" | "rss" | "rtprio" | "rttime" | "sigpending" | "stack"

				softLimit!: int

				hardLimit!: int
			})]

			logConfiguration?: close({
				logDriver!: "json-file" | "syslog" | "journald" | "gelf" | "fluentd" | "awslogs" | "splunk" | "awsfirelens"

				options?: {
					"insert-key"?: string
					...
				}

				secretOptions?: [...close({
					name!: string

					valueFrom!: string
				})]
			})

			healthCheck?: close({
				command!: [...string]

				interval?: int

				timeout?: int

				retries?: int

				startPeriod?: int
			})

			systemControls?: [...close({
				namespace?: string

				value?: string
			})]

			resourceRequirements?: [...close({
				value!: string

				type!: "GPU" | "InferenceAccelerator"
			})]

			firelensConfiguration?: close({
				type!: "fluentd" | "fluentbit"

				options?: {
					"insert-key"?: string
					...
				}
			})
		})]

		volumes?: [...close({
			name?: string

			host?: close({
				sourcePath?: string
			})

			dockerVolumeConfiguration?: close({
				scope?: "task" | "shared"

				autoprovision?: bool

				driver?: string

				driverOpts?: {
					"insert-key"?: string
					...
				}

				labels?: {
					"insert-key"?: string
					...
				}
			})

			efsVolumeConfiguration?: close({
				fileSystemId!: string

				rootDirectory?: string

				transitEncryption?: "ENABLED" | "DISABLED"

				transitEncryptionPort?: int

				authorizationConfig?: close({
					accessPointId?: string

					iam?: "ENABLED" | "DISABLED"
				})
			})
		})]

		placementConstraints?: [...close({
			type?: "memberOf"

			expression?: string
		})]

		requiresCompatibilities?: [..."EC2" | "FARGATE"]

		cpu?: string

		memory?: string

		tags?: [...close({
			key?: strings.MaxRunes(128) & strings.MinRunes(1) & =~"^([\\p{L}\\p{Z}\\p{N}_.:/=+\\-@]*)$"

			value?: strings.MaxRunes(256) & strings.MinRunes(0) & =~"^([\\p{L}\\p{Z}\\p{N}_.:/=+\\-@]*)$"
		})]

		pidMode?: "host" | "task"

		ipcMode?: "host" | "task" | "none"

		proxyConfiguration?: close({
			type?: "APPMESH"

			containerName!: string

			properties?: [...close({
				name?: string

				value?: string
			})]
		})

		inferenceAccelerators?: [...close({
			deviceName!: string

			deviceType!: string
		})]

		runtimePlatform?: close({
			cpuArchitecture?:       "X86_64" | "ARM64"
			operatingSystemFamily?: "LINUX" | "WINDOWS_SERVER_2019_FULL" | "WINDOWS_SERVER_2019_CORE" | "WINDOWS_SERVER_2022_FULL" | "WINDOWS_SERVER_2022_CORE"
		})

		ephemeralStorage?: close({
			sizeInGiB!: int & >=21 & <=200
		})

		enableFaultInjection?: bool
	})
}
