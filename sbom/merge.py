import json
import sys

# Get the filenames from command line arguments
filenames = sys.argv[1:]

base_sbom = {}

# Merge everything into the first provided sbom
for idx, filename in enumerate(filenames):
    with open(filename, 'r') as file:
        sbom = json.load(file)
        
        if idx == 0:
            base_sbom = sbom
        else:
            appRef = sbom["metadata"]["component"]["bom-ref"]
            baseAppRef = base_sbom["metadata"]["component"]["bom-ref"]

            for didx, dep in enumerate(base_sbom["dependencies"]):
                if dep["ref"] == baseAppRef:
                    dep["dependsOn"].append(appRef)

            base_sbom["components"].append(sbom["metadata"]["component"])
            base_sbom["components"] += sbom["components"]
            base_sbom["dependencies"] += sbom["dependencies"]

# Set groups
for idx, cp in enumerate(base_sbom["components"]):
    if "purl" in cp:
        if cp["purl"].startswith("pkg:golang"):
            cp["group"] = "gomod"

        if cp["purl"].startswith("pkg:npm"):
            cp["group"] = "npm"

        if cp["purl"].startswith("pkg:deb"):
            cp["group"] = "deb"
            
    if "type" in cp and cp["type"] == "application":
        cp["group"] = "application"

    base_sbom["components"][idx] = cp

print(json.dumps(base_sbom, indent=4))
