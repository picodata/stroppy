#!/bin/bash
# Usage: 
# ./get_png.sh 1622733157321 1622744447611
# where $1 start in nuix time, $2 end in nuix time
# vars
rm -rf png

start=$1
end=$2
arch_name=$3
ip_string=$4
base_url="http://admin:admin@localhost:3000/render/d-solo"
tz="tz=Europe%2FMoscow"

ip_array=($(echo $ip_string | tr ";" "\n"))

for theme in "light" "dark"
do
    for size in "width=3000&height=1800" "width=1000&height=500"
    do
        mkdir -p "png/$size/$theme/node-exporter"
        mkdir -p "png/$size/$theme/k8s"
        i=1
        for worker in "${ip_array[@]}"
        do

            curl -s -o png/$size/$theme/node-exporter/cpu-worker-$i.png                     "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=77&$size&$tz"  
            curl -s -o png/$size/$theme/node-exporter/cpu-details-worker-$i.png             "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=3&$size&$tz"  
            curl -s -o png/$size/$theme/node-exporter/ram-worker-$i.png                     "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=78&$size&$tz"  
            curl -s -o png/$size/$theme/node-exporter/ram-details-worker-$i.png             "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=24&$size&$tz"            
            curl -s -o png/$size/$theme/node-exporter/network-traffic-worker-$i.png         "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=74&$size&$tz"
            curl -s -o png/$size/$theme/node-exporter/netstat-in-out-octets-worker-$i.png   "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=221&$size&$tz" 
            curl -s -o png/$size/$theme/node-exporter/network-in-out-udp-worker-$i.png      "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=55&$size&$tz"  
            curl -s -o png/$size/$theme/node-exporter/network-in-out-tcp-worker-$i.png      "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=299&$size&$tz" 
            curl -s -o png/$size/$theme/node-exporter/disk-space-used-worker-$i.png         "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=156&$size&$tz" 
            curl -s -o png/$size/$theme/node-exporter/disk-iops-worker-$i.png               "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=229&$size&$tz"
            curl -s -o png/$size/$theme/node-exporter/disk-io-usage-rw-worker-$i.png        "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=42&$size&$tz"  
            curl -s -o png/$size/$theme/node-exporter/disk-io-utilization-worker-$i.png     "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=127&$size&$tz" 
            curl -s -o png/$size/$theme/node-exporter/disk-average-wait-time-worker-$i.png  "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=37&$size&$tz"  
            curl -s -o png/$size/$theme/node-exporter/disk-rw-merged-worker-$i.png          "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=133&$size&$tz" 
            curl -s -o png/$size/$theme/node-exporter/disk-average-queue-size-worker-$i.png "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=35&$size&$tz"
            curl -s -o png/$size/$theme/node-exporter/system-load-worker-$i.png             "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=7&$size&$tz"   
            curl -s -o png/$size/$theme/node-exporter/cpu-context-switches-worker-$i.png    "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=8&$size&$tz"   
            curl -s -o png/$size/$theme/node-exporter/ram-active-inactive-worker-$i.png     "$base_url/rYdddlPWk/node-exporter-full?orgId=1&from=$start&to=$end&var-job=node-exporter&var-node=$worker:9100&var-diskdevices=%5Ba-z%5D%2B%7Cnvme%5B0-9%5D%2Bn%5B0-9%5D%2B&theme=$theme&panelId=191&$size&$tz"
            let i++
        done
        curl -s -o png/$size/$theme/k8s/k8s-cpu-by-namespaces.png                       "$base_url/efa86fd1d0c121a26444b636a3f509a8/kubernetes-compute-resources-cluster?orgId=1&from=$start&to=$end&theme=$theme&panelId=7&$size&$tz"                                                       
        curl -s -o png/$size/$theme/k8s/k8s-memory-by-namespaces.png                    "$base_url/efa86fd1d0c121a26444b636a3f509a8/kubernetes-compute-resources-cluster?orgId=1&from=$start&to=$end&theme=$theme&panelId=9&$size&$tz"                                                       
        curl -s -o png/$size/$theme/k8s/k8s-cpu-by-pods-in-default-namespace.png        "$base_url/85a562078cdf77779eaa1add43ccec1e/kubernetes-compute-resources-namespace-pods?orgId=1&from=$start&to=$end&theme=$theme&panelId=5&$size&$tz"                                                
        curl -s -o png/$size/$theme/k8s/k8s-memory-by-pods-in-default-namespace.png     "$base_url/85a562078cdf77779eaa1add43ccec1e/kubernetes-compute-resources-namespace-pods?orgId=1&from=$start&to=$end&theme=$theme&panelId=7&$size&$tz"                                                
        # warning: 'var-node=worker-1&var-node=worker-2&var-node=worker-3' block is variable
        curl -s -o png/$size/$theme/k8s/k8s-cpu-by-all-pods.png                         "$base_url/200ac8fdbfbb74b39aff88118e4d1c2c/kubernetes-compute-resources-node-pods?orgId=1&var-datasource=default&var-cluster=&var-node=worker-1&var-node=worker-2&var-node=worker-3&from=$start&to=$end&theme=$theme&panelId=1&$size&$tz"
        curl -s -o png/$size/$theme/k8s/k8s-memory-by-all-pods.png                      "$base_url/200ac8fdbfbb74b39aff88118e4d1c2c/kubernetes-compute-resources-node-pods?orgId=1&var-datasource=default&var-cluster=&var-node=worker-1&var-node=worker-2&var-node=worker-3&from=$start&to=$end&theme=$theme&panelId=3&$size&$tz"          
        curl -s -o png/$size/$theme/k8s/k8s-cpu-by-sts.png                              "$base_url/a164a7f0339f99e89cea5cb47e9be617/kubernetes-compute-resources-workload?orgId=1&from=$start&to=$end&theme=$theme&panelId=1&$size&$tz"                                                                 
        curl -s -o png/$size/$theme/k8s/k8s-memory-by-sts.png                           "$base_url/a164a7f0339f99e89cea5cb47e9be617/kubernetes-compute-resources-workload?orgId=1&from=$start&to=$end&theme=$theme&panelId=3&$size&$tz"                                                                 
        curl -s -o png/$size/$theme/k8s/k8s-net-receive-by-namespaces.png               "$base_url/ff635a025bcfea7bc3dd4f508990a3e9/kubernetes-networking-cluster?orgId=1&var-resolution=30s&var-interval=4h&var-datasource=default&var-cluster=&from=$start&to=$end&theme=$theme&panelId=10&$size&$tz" 
        curl -s -o png/$size/$theme/k8s/k8s-net-transmit-by-namespaces.png              "$base_url/ff635a025bcfea7bc3dd4f508990a3e9/kubernetes-networking-cluster?orgId=1&var-resolution=30s&var-interval=4h&var-datasource=default&var-cluster=&from=$start&to=$end&theme=$theme&panelId=11&$size&$tz"
        curl -s -o png/$size/$theme/k8s/k8s-net-receive-by-all-pods.png                 "$base_url/8b7a8b326d7a6f1f04244066368c67af/kubernetes-networking-namespace-pods?orgId=1&var-datasource=default&var-cluster=&var-namespace=All&var-resolution=30s&var-interval=4h&from=$start&to=$end&theme=$theme&panelId=7&$size&$tz"                               
        curl -s -o png/$size/$theme/k8s/k8s-net-transmit-by-all-pods.png                "$base_url/8b7a8b326d7a6f1f04244066368c67af/kubernetes-networking-namespace-pods?orgId=1&var-datasource=default&var-cluster=&var-namespace=All&var-resolution=30s&var-interval=4h&from=$start&to=$end&theme=$theme&panelId=8&$size&$tz"                               
        curl -s -o png/$size/$theme/k8s/k8s-net-receive-by-sts.png                      "$base_url/bbb2a765a623ae38130206c7d94a160f/kubernetes-networking-namespace-workload?orgId=1&var-datasource=default&var-cluster=&var-namespace=default&var-type=statefulset&var-resolution=30s&var-interval=4h&from=$start&to=$end&theme=$theme&panelId=10&$size&$tz" 
        curl -s -o png/$size/$theme/k8s/k8s-net-transmit-by-sts.png                     "$base_url/bbb2a765a623ae38130206c7d94a160f/kubernetes-networking-namespace-workload?orgId=1&var-datasource=default&var-cluster=&var-namespace=default&var-type=statefulset&var-resolution=30s&var-interval=4h&from=$start&to=$end&theme=$theme&panelId=11&$size&$tz" 
        curl -s -o png/$size/$theme/k8s/k8s-net-receive-by-pods-in-sts.png              "$base_url/728bf77cc1166d2f3133bf25846876cc/kubernetes-networking-workload?orgId=1&var-datasource=default&var-cluster=&var-namespace=All&var-workload=acid-postgres-cluster&var-type=statefulset&var-resolution=30s&var-interval=4h&from=$start&to=$end&theme=$theme&panelId=9&$size&$tz"
        curl -s -o png/$size/$theme/k8s/k8s-net-transmit-by-pods-in-sts.png             "$base_url/728bf77cc1166d2f3133bf25846876cc/kubernetes-networking-workload?orgId=1&var-datasource=default&var-cluster=&var-namespace=All&var-workload=acid-postgres-cluster&var-type=statefulset&var-resolution=30s&var-interval=4h&from=$start&to=$end&theme=$theme&panelId=10&$size&$tz"
    done
done

mv 'png/width=1000&height=500'  png/1000x500
mv 'png/width=3000&height=1800' png/3000x1800

tar cfvz $arch_name png
