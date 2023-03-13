package internal

type Pod struct {
	podName string
}

type PodRef struct {
	PodRef struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"podRef"`
	EphemeralStorage struct {
		Usedbytes float64 `json:"usedBytes"`
	} `json:"ephemeral-storage"`
}

type Node struct {
	Node struct {
		NodeName string `json:"nodeName"`
	} `json:"node"`
	Pods []PodRef `json:"pods"`
}