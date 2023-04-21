import json
import sys
import uuid
import datetime
import yaml

# Get the filenames from command line arguments
filenames = sys.argv[1:]

baseRef = str(uuid.uuid4())
sbom = {
    "bomFormat": "CycloneDX",
    "specVersion": "1.4",
    "version": 1,
    "metadata": {
        "component": {
            "bom-ref": baseRef,
            "type": "file",
            "name": filenames[0],
        }
    },
    "components":[],
    "dependencies":[]
}

sbom["serialNumber"] = "urn:uuid:" + str(uuid.uuid4())
sbom["metadata"]["timestamp"] = datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S+00:00")

depRefs = []

with open(filenames[0], 'r') as file:
    spec = yaml.safe_load(file)
    for dep in spec["spec"]["dependsOnExt"]:
        for v in dep["versions"]:
            depUUID = str(uuid.uuid4())
            depRefs.append(depUUID)
            sbom["components"].append({
                "bom-ref": depUUID,
                "type": "application",
                "name": (dep["name"] + " " + str(v)),
                "version": str(v),
                "cpe": (dep["cpe"] + ":" + str(v))
            })

sbom["dependencies"] = [
    {
        "ref": baseRef,
        "dependsOn": depRefs
    }
]

print(json.dumps(sbom, indent=4)) 
