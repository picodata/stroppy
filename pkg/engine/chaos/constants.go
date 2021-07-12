package chaos

const (
	chaosNamespace             = "chaos-testing"
	chaosDashboardResourceName = "chaos-dashboard"

	chaosSshEntity = "chaos"

	deployChaosMesh = `
helm repo add chaos-mesh https://charts.chaos-mesh.org
helm search repo chaos-mesh
kubectl create ns chaos-testing
helm install chaos-mesh chaos-mesh/chaos-mesh --namespace=chaos-testing
helm upgrade chaos-mesh chaos-mesh/chaos-mesh --namespace=chaos-testing --set dashboard.create=true
`
)
