package kubernetes

#Meta: {
    id:         "k8s:apps:deployment"
    name:       "Kubernetes Deployment"
    version:    "1.0"
    matchPatterns: ["deployment*.yaml", "deployment*.yml", "deploy*.yaml"]
}

#Schema: {
    apiVersion!: "apps/v1"
    kind!:       "Deployment"

    metadata!: {
        name!:        string & =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
        namespace?:   string | *"default"
        labels?:      [string]: string
        annotations?: [string]: string
    }

    spec!: {
        replicas?: int & >=0 | *1

        selector!: {
            matchLabels!: [string]: string & {[_]: _}  // At least one
        }

        template!: {
            metadata!: {
                labels!: [string]: string
            }
            spec!: {
                containers!: [...#Container] & [_, ...]
                initContainers?: [...#Container]
                serviceAccountName?: string
                restartPolicy?: *"Always" | "OnFailure" | "Never"
            }
        }

        // selector.matchLabels must match template.metadata.labels
        selector: matchLabels: template.metadata.labels
    }
}

#Container: {
    name!:  string & =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
    image!: string

    ports?: [...{
        containerPort!: int & >0 & <=65535
        protocol?:      *"TCP" | "UDP" | "SCTP"
        name?:          string
    }]

    env?: [...{
        name!:  string
        value?: string
        valueFrom?: {
            secretKeyRef?: {name!: string, key!: string}
            configMapKeyRef?: {name!: string, key!: string}
        }
    }]

    resources?: {
        limits?:   {cpu?: string, memory?: string}
        requests?: {cpu?: string, memory?: string}
    }

    livenessProbe?:  #Probe
    readinessProbe?: #Probe
}

#Probe: {
    httpGet?:   {path!: string, port!: int | string}
    tcpSocket?: {port!: int | string}
    exec?:      {command!: [...string]}
    initialDelaySeconds?: int | *0
    periodSeconds?:       int | *10
    timeoutSeconds?:      int | *1
    failureThreshold?:    int | *3
}