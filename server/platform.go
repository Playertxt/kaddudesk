package main

// Monitor is one capturable screen. Adapter/Output are DXGI indices (Windows only).
type Monitor struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Adapter int    `json:"adapter"`
	Output  int    `json:"output"`
	X       int32  `json:"x"`
	Y       int32  `json:"y"`
	W       int32  `json:"w"`
	H       int32  `json:"h"`
	Primary bool   `json:"primary"`
	GPU     string `json:"gpu"`
}
