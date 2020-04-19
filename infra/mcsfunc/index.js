'use strict';

// https://cloud.google.com/compute/docs/regions-zones
const gcpRegions = {
  "asia-east1": { "zones": ["a", "b", "c"], "location": "Changhua County, Taiwan" },
  "asia-east2": { "zones": ["a", "b", "c"], "location": "Hong Kong" },
  "asia-northeast1": { "zones": ["a", "b", "c"], "location": "Tokyo, Japan" },
  "asia-northeast2": { "zones": ["a", "b", "c"], "location": "Osaka, Japan" },
  "asia-northeast3": { "zones": ["a", "b", "c"], "location": "Seoul, South Korea" },
  "asia-south1": { "zones": ["a", "b", "c"], "location": "Mumbai, India" },
  "asia-southeast1": { "zones": ["a", "b", "c"], "location": "Jurong West, Singapore" },
  "australia-southeast1": { "zones": ["a", "b", "c"], "location": "Sydney, Australia" },
  "europe-north1": { "zones": ["a", "b", "c"], "location": "Hamina, Finland" },
  "europe-west1": { "zones": ["b", "c", "d"], "location": "St. Ghislain, Belgium" },
  "europe-west2": { "zones": ["a", "b", "c"], "location": "London, England, UK" },
  "europe-west3": { "zones": ["a", "b", "c"], "location": "Frankfurt, Germany" },
  "europe-west4": { "zones": ["a", "b", "c"], "location": "Eemshaven, Netherlands" },
  "europe-west6": { "zones": ["a", "b", "c"], "location": "Zürich, Switzerland" },
  "northamerica-northeast1": { "zones": ["a", "b", "c"], "location": "Montreal, Quebec, Canada" },
  "southamerica-east1": { "zones": ["a", "b", "c"], "location": "Osasco (Sao Paulo), Brazil" },
  "us-central1": { "zones": ["a", "b", "c", "f"], "location": "Council Bluffs, Iowa, USA" },
  "us-east1": { "zones": ["b", "c", "d"], "location": "Moncks Corner, South Carolina, USA" },
  "us-east4": { "zones": ["a", "b", "c"], "location": "Ashburn, Northern Virginia, USA" },
  "us-west1": { "zones": ["a", "b", "c"], "location": "The Dalles, Oregon, USA" },
  "us-west2": { "zones": ["a", "b", "c"], "location": "Los Angeles, California, USA" },
  "us-west3": { "zones": ["a", "b", "c"], "location": "Salt Lake City, Utah, USA" },
}


const createMcsVMConfig = {
  os: "ubuntu",
  http: true,
  machineType: "g1-small",
  scheduling: {
    preemptible: true
  },
  metadata: {
    items: [
      {
        key: "startup-script",
        value: `\
#!/bin/bash

apt-get update
apt-get install -y jq wget curl

rm -f /etc/systemd/system/gdxsv-mcs.service
cat << 'EOF' > /etc/systemd/system/gdxsv-mcs.service
[Unit]
Description=gdxsv mcs service
After=systemd-networkd-wait-online.service

[Service]
Restart=on-failure
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu
ExecStart=/home/ubuntu/launch-mcs.sh

[Install]
WantedBy=multi-user.target
EOF

cat << 'EOF' > /home/ubuntu/launch-mcs.sh
#!/bin/bash

set -eux

function finish {
  echo "finish"
  sleep 1
  sudo /sbin/shutdown now
}
trap finish EXIT

if grep -xqFe 'ubuntu ALL=NOPASSWD: /sbin/shutdown' /etc/sudoers; then
  echo 'ubuntu ALL=NOPASSWD: /sbin/shutdown' >> /etc/sudoers"
fi

readonly LATEST_TAG=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.tag_name')
readonly DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.assets[].browser_download_url')

if [[ ! -d $LATEST_TAG/bin ]]; then
  echo "Downloading latest version..."
  mkdir -p $LATEST_TAG
  pushd $LATEST_TAG
    wget $DOWNLOAD_URL
    tar xzvf bin.tgz && rm bin.tgz
  popd
fi

readonly GCP_NAT_IP=$(curl -H "Metadata-Flavor: Google" http://metadata/computeMetadata/v1/instance/network-interfaces/0/ip)
readonly GCP_ZONE=$(basename $(curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/zone))

export GDXSV_LOBBY_PUBLIC_ADDR="zdxsv.net:9876"
export GDXSV_BATTLE_ADDR=":9877"
export GDXSV_BATTLE_ZONE=\${GCP_ZONE}
export GDXSV_BATTLE_PUBLIC_ADDR="\${GCP_NAT_IP}:9877"
$LATEST_TAG/bin/gdxsv mcs -v=3
EOF

chmod +x /home/ubuntu/launch-mcs.sh

systemctl daemon-reload
systemctl enable systemd-networkd
systemctl enable systemd-networkd-wait-online
systemctl enable gdxsv-mcs
systemctl start gdxsv-mcs --no-block
`
      },
    ],
  },
}

function forResponse(vm) {
  const v = {};
  try {
    v.name = vm.name;
    v.zone = vm.metadata.zone;
    v.created = vm.metadata.creationTimestamp;
    v.status = vm.metadata.status;
    v.tags = vm.metadata.tags.items;
    v.nat_ip = vm.metadata.networkInterfaces[0].accessConfigs[0].natIP;
  } catch (e) {
    console.log(e);
  }
  return v;
}

// require GOOGLE_APPLICATION_CREDENTIALS environment variable
const Compute = require('@google-cloud/compute');
const url = require('url');



exports.cloudFunctionEntryPoint = async (req, res) => {
  console.log(req.url);

  const query = url.parse(req.url, true).query
  const compute = new Compute();

  if (req.method != "GET") {
    res.status(400).send('bad request');
    return;
  }

  if (req.url.startsWith("/list")) {
    const [vms] = await compute.getVMs({
      autoPaginate: false,
      maxResults: 100,
      filter: "name eq gdxsv-mcs",
    });

    const vmlist = [];
    for (let i = 0; i < vms.length; i++) {
      vmlist.push(forResponse(vms[i]));
    }

    res.setHeader('Content-Type', 'application/json');
    res.send(JSON.stringify(vmlist, null, "  "));
    return;
  }

  if (req.url.startsWith("/regions")) {
    res.setHeader('Content-Type', 'application/json');
    res.send(JSON.stringify(gcpRegions, null, "  "));
    return;
  }

  if (req.url.startsWith("/deleteall")) {
    const [vms] = await compute.getVMs({
      autoPaginate: false,
      maxResults: 100,
      filter: "name eq gdxsv-mcs",
    });

    const vmlist = [];
    const deletes = [];
    for (let i = 0; i < vms.length; i++) {
      const [operation] = await vms[i].delete()
      deletes.push(operation.promise);
      vmlist.push(forResponse(vms[i]));
    }
    await Promise.all(deletes);

    res.setHeader('Content-Type', 'application/json');
    res.send(JSON.stringify(vmlist, null, "  "));
    return;
  }

  if (req.url.startsWith("/alloc")) {
    const region = query["region"];
    const regionInfo = gcpRegions[region];

    if (!regionInfo) {
      res.status(400).send('invalid region');
      return;
    }

    let [vms] = await compute.getVMs({
      autoPaginate: false,
      maxResults: 100,
      filter: "name eq gdxsv-mcs",
    })
    vms = vms.filter(vm => vm.metadata.zone.includes(region));

    console.log("" + vms.length + "vms found.");

    let vm = vms.find(vm => vm.metadata.status == "RUNNING");
    if (vm) {
      res.setHeader('Content-Type', 'application/json');
      res.send(JSON.stringify(forResponse(vm), null, "  "));
      return;
    }

    console.log("running vm not found");

    for (let vm of vms.filter(vm => vm.metadata.status == "TERMINATED")){
      try {
        console.log("starting vm...", vm);
        const [operation] = await vm.start();
        await operation.promise();
        console.log("start vm done");
        [vm.metadata] = await vm.waitFor("RUNNING", { timeout: 30 });
        console.log("wait done");
      } catch (e) {
        console.log(e);
        continue;
      }
      res.setHeader('Content-Type', 'application/json');
      res.send(JSON.stringify(forResponse(vm), null, "  "));
      return;
    }

    console.log("no available vm found in", region);

    for (let z of regionInfo.zones) {
      const zoneName = region + "-" + z;
      try {
        console.log("trying to create new vm in", zoneName);
        const zone = compute.zone(zoneName);
        const [vm, operation] = await zone.createVM("gdxsv-mcs", createMcsVMConfig);
        await operation.promise();
        console.log("vm created");
        [vm.metadata] = await vm.waitFor("RUNNING", { timeout: 30 });
        console.log("wait done");
      } catch (e) {
        console.log(e);
        continue;
      }
      res.setHeader('Content-Type', 'application/json');
      res.send(JSON.stringify(forResponse(vm), null, "  "));
      return;
    }

    console.log('failed to allocate vm');
    res.status(503).send('failed to allocate vm');
    return;
  }

  res.status(400).send('bad request');
  return;
}