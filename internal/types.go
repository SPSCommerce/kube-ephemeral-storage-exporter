package internal

type Pod struct {
	PodName string
}

type PodRef struct {
	PodRef struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"podRef"`
	EphemeralStorage struct {
		Usedbytes any `json:"usedBytes"`
	} `json:"ephemeral-storage"`
}

type Node struct {
	Node struct {
		NodeName string `json:"nodeName"`
	} `json:"node"`
	Pods []PodRef `json:"pods"`
}
