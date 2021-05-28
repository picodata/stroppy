package chaos

const deployChaosMesh = `
helm install chaos-mesh chaos-mesh/chaos-mesh --namespace=chaos-testing
helm upgrade chaos-mesh chaos-mesh/chaos-mesh --namespace=chaos-testing --set dashboard.create=true
`
