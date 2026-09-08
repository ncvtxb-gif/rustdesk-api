package api

type ManagedDeviceAuthenticationHash struct {
	Hash string `json:"hash" validate:"required" label:"hash"`
}

type ManagedDeviceBootstrap struct {
	MachineUUID string `json:"machine_uuid" validate:"required" label:"machine_uuid"`
	Platform    string `json:"platform" validate:"required" label:"platform"`
}
