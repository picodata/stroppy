package oracle

type TfState struct{
    Data []byte
    Outputs Outputs `json:"outputs"`
}

type Outputs struct {
    InstancePublicIps InstanceIps `json:"instance_public_ips"`
    InstancePrivateIps InstanceIps `json:"instance_private_ips"`
}

type InstanceIps struct {
    Value [][]string
}
