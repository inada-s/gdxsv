'use strict';

const Compute = require('@google-cloud/compute');
const url = require('url');

// https://cloud.google.com/compute/docs/regions-zones
const gcpRegions = {
    "asia-east1": {"zones": ["a", "b", "c"], "location": "Changhua County, Taiwan"},
    "asia-east2": {"zones": ["a", "b", "c"], "location": "Hong Kong"},
    "asia-northeast1": {"zones": ["a", "b", "c"], "location": "Tokyo, Japan"},
    "asia-northeast2": {"zones": ["a", "b", "c"], "location": "Osaka, Japan"},
    "asia-northeast3": {"zones": ["a", "b", "c"], "location": "Seoul, South Korea"},
    "asia-south1": {"zones": ["a", "b", "c"], "location": "Mumbai, India"},
    "asia-southeast1": {"zones": ["a", "b", "c"], "location": "Jurong West, Singapore"},
    "australia-southeast1": {"zones": ["a", "b", "c"], "location": "Sydney, Australia"},
    "europe-north1": {"zones": ["a", "b", "c"], "location": "Hamina, Finland"},
    "europe-west1": {"zones": ["b", "c", "d"], "location": "St. Ghislain, Belgium"},
    "europe-west2": {"zones": ["a", "b", "c"], "location": "London, England, UK"},
    "europe-west3": {"zones": ["a", "b", "c"], "location": "Frankfurt, Germany"},
    "europe-west4": {"zones": ["a", "b", "c"], "location": "Eemshaven, Netherlands"},
    "europe-west6": {"zones": ["a", "b", "c"], "location": "Zürich, Switzerland"},
    "northamerica-northeast1": {"zones": ["a", "b", "c"], "location": "Montreal, Quebec, Canada"},
    "southamerica-east1": {"zones": ["a", "b", "c"], "location": "Osasco (Sao Paulo), Brazil"},
    "us-central1": {"zones": ["a", "b", "c", "f"], "location": "Council Bluffs, Iowa, USA"},
    "us-east1": {"zones": ["b", "c", "d"], "location": "Moncks Corner, South Carolina, USA"},
    "us-east4": {"zones": ["a", "b", "c"], "location": "Ashburn, Northern Virginia, USA"},
    "us-west1": {"zones": ["a", "b", "c"], "location": "The Dalles, Oregon, USA"},
    "us-west2": {"zones": ["a", "b", "c"], "location": "Los Angeles, California, USA"},
    "us-west3": {"zones": ["a", "b", "c"], "location": "Salt Lake City, Utah, USA"},
}

function getStartupScript(version) {
    return `\
#!/bin/bash
echo "startup-script"

apt-get update
apt-get install -y jq wget curl

if [[ ! -e install-monitoring-agent.sh ]]; then
  curl -sSO https://dl.google.com/cloudagents/install-monitoring-agent.sh
  bash install-monitoring-agent.sh
fi

if [[ ! -e install-logging-agent.sh ]]; then
  curl -sSO https://dl.google.com/cloudagents/install-logging-agent.sh
  bash install-logging-agent.sh
fi

if grep -xqFe 'ubuntu ALL=NOPASSWD: /sbin/shutdown' /etc/sudoers; then
  echo 'ubuntu ALL=NOPASSWD: /sbin/shutdown' >> /etc/sudoers
fi

cat << 'EOF' > /etc/google-fluentd/config.d/gdxsv.conf
<source>
  @type tail
  format json
  path /var/log/gdxsv-mcs.log
  pos_file /var/lib/google-fluentd/pos/gdxsv.pos
  read_from_head true
  tag gdxsv-mcs
</source>
EOF
sudo systemctl restart google-fluentd


cat << 'EOF' > /home/ubuntu/launch-mcs.sh
#!/bin/bash -eux

function finish {
  echo "mcs finished" | logger
  sleep 1
  sudo /sbin/shutdown now
}
trap finish EXIT

readonly VERSION=${version}

if [[ -z $VERSION || $VERSION == "latest" ]]; then
  readonly TAG_NAME=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.tag_name')
  readonly DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.assets[].browser_download_url')
else
  readonly TAG_NAME=$VERSION
  readonly DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/tags/$TAG_NAME | jq -r '.assets[].browser_download_url')
fi

if [[ ! -d $TAG_NAME/bin ]]; then
  echo "Downloading $TAG_NAME"
  mkdir -p "$TAG_NAME"
  pushd "$TAG_NAME"
    wget "$DOWNLOAD_URL"
    tar xzvf bin.tgz && rm bin.tgz
  popd
fi

export GDXSV_LOBBY_PUBLIC_ADDR=zdxsv.net:9876
export GDXSV_BATTLE_ADDR=:9877
export GDXSV_BATTLE_REGION=$(basename $(curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/zone))
export GDXSV_BATTLE_PUBLIC_ADDR=$(curl -s https://ipinfo.io/ip):9877

"$TAG_NAME"/bin/gdxsv -prodlog mcs >> /var/log/gdxsv-mcs.log 2>&1
EOF

touch /var/log/gdxsv-mcs.log
truncate -s0 /var/log/gdxsv-mcs.log
chown ubuntu:ubuntu /var/log/gdxsv-mcs.log

chmod +x /home/ubuntu/launch-mcs.sh
su ubuntu -c 'cd /home/ubuntu && nohup ./launch-mcs.sh &'
echo "startup-script done"
`
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

async function getList(req, res) {
    const compute = new Compute();

    const [vms] = await compute.getVMs({
        autoPaginate: false,
        maxResults: 100,
        filter: "name eq gdxsv-mcs.*",
    });

    const vmlist = [];
    for (let i = 0; i < vms.length; i++) {
        vmlist.push(forResponse(vms[i]));
    }

    res.setHeader('Content-Type', 'application/json');
    res.send(JSON.stringify(vmlist, null, "  "));
    return;
}

async function getDeleteAll(req, res) {
    const compute = new Compute();

    const [vms] = await compute.getVMs({
        autoPaginate: false,
        maxResults: 100,
        filter: "name eq gdxsv-mcs.*",
    });

    const vmlist = [];
    const deletes = [];
    for (let vm of vms) {
        const [operation] = await vm.delete()
        deletes.push(operation.promise);
        vmlist.push(forResponse(vm));
    }
    await Promise.all(deletes);

    res.setHeader('Content-Type', 'application/json');
    res.send(JSON.stringify(vmlist, null, "  "));
}


async function getAlloc(req, res) {
    const compute = new Compute();
    const query = url.parse(req.url, true).query

    const region = query["region"];
    const version = query["version"] ? query["version"] : "latest";
    const regionInfo = gcpRegions[region];
    const vmName = "gdxsv-mcs-" + region + "-" + version.replace(/\./g, "-")

    if (!regionInfo) {
        res.status(400).send('invalid region');
        return;
    }

    let [vms] = await compute.getVMs({
        autoPaginate: false,
        maxResults: 100,
        filter: "name eq " + vmName,
    })

    console.log("" + vms.length + "vms found.");

    let vm = vms.find(vm => vm.metadata.status == "RUNNING");
    if (vm) {
        res.setHeader('Content-Type', 'application/json');
        res.send(JSON.stringify(forResponse(vm), null, "  "));
        return;
    }

    console.log("running vm not found");

    for (let vm of vms.filter(vm => vm.metadata.status == "TERMINATED")) {
        try {
            console.log("starting vm...", vm);
            let [operation] = await vm.setMetadata({
                'startup-script': getStartupScript(version),
            });
            await operation.promise();
            [operation] = await vm.start();
            await operation.promise();
            console.log("start vm done");
            [vm.metadata] = await vm.waitFor("RUNNING", {timeout: 30});
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
            const [vm, operation] = await zone.createVM(vmName, {
                os: "ubuntu",
                http: true,
                tags: ["gdxsv-mcs"],
                machineType: "g1-small",
                scheduling: {preemptible: true},
                metadata: {
                    items: [
                        {key: "startup-script", value: getStartupScript(version)},
                    ],
                },
                serviceAccounts: [
                    {
                        email: "gdxsv-service@gdxsv-274515.iam.gserviceaccount.com",
                        scopes: [
                            "https://www.googleapis.com/auth/logging.write"
                        ]
                    }
                ]
            });

            await operation.promise();
            console.log("vm created");
            const [metadata] = await vm.waitFor("RUNNING", {timeout: 30});
            vm.metadata = metadata
            console.log("new vm is running", vm);
            res.setHeader('Content-Type', 'application/json');
            res.send(JSON.stringify(forResponse(vm), null, "  "));
            return;
        } catch (e) {
            console.log(e);
            continue;
        }
    }

    console.log('failed to allocate vm');
    res.status(503).send('failed to allocate vm');
    return;
}


exports.cloudFunctionEntryPoint = async (req, res) => {
    if (req.method != "GET") {
        res.status(400).send('bad request');
        return;
    }

    if (req.url.startsWith("/list")) {
        return await getList(req, res);
    }
    if (req.url.startsWith("/deleteall")) {
        return await getDeleteAll(req, res);
    }
    if (req.url.startsWith("/alloc")) {
        return await getAlloc(req, res);
    }

    res.status(400).send('bad request');
}