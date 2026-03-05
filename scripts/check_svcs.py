import json, sys
s = json.load(open("/tmp/svcs.json"))
k = [x for x in s if x["category"] != "unknown"]
print(f"{len(s)} total, {len(k)} known")
for x in k:
    print(f"  {x['name']:20s} :{x['port']}/{x['protocol']}  [{x['category']}] via {x['source']}")
