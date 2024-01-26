import { Cosmograph } from '@cosmograph/cosmograph'
import jsonData from "./nomad-json.json";

// Types
type Node = {
    id: string;
    color: string;
    label: string;
    x?: number;
    y?: number;
  }
  
  type Link = {
    source: string;
    target: string;
  }


// Load data from json
const data: any = jsonData;

let nodes: Node[] = [];
let links: Link[] = [];

data.findings.forEach((finding: any) => {
    finding.Paths.forEach((paths: string[]) => {
        let prev = "";
        paths.forEach((path, i) => {
            if (i === 0) {
                prev = path;
                addNodeIfNotExist(path, "#984040");
            } else {
                addNodeIfNotExist(path, "#4287f5");
                addEdgeIfNotExist(path, prev);
                prev = path;
            }
        });
    });
});

function nodeExists(nodeId: string): boolean {
    return nodes.some(node => node.id === nodeId);
}

function edgeExists(source: string, target: string): boolean {
    return links.some(link => link.source === source && link.target === target);
}

function addNodeIfNotExist(nodeId: string, color: string) {
    if (!nodeExists(nodeId)) {
        let label = extractFuncFromId(nodeId)
        label = label != null ? label : ""
        nodes.push({ id: nodeId, label: label, color: color });
    }
}

function addEdgeIfNotExist(source: string, target: string) {
    if (!edgeExists(source, target)) {
        links.push({ source, target });
    }
}

function extractFuncFromId(nodeId: string): string | null {
    const match = nodeId.match(/\[(.*?)\]/);
    return match ? match[1] : null;
}

// const canvas = document.getElementById("container")

const canvas = document.createElement('div')
document.body.appendChild(canvas)

const config = {
    // backgroundColor: "#151515",
    nodeSize: 3,
    nodeColor: (n, i) => n.color,
    linkWidth: 0.5,
    linkColor: "#8C8C8C",
    linkArrows: true,
    nodeLabelAccessor: n => n.label,
    simulation: {
      repulsion: 0.5,
    },
    // renderLinks: true,
    events: {
      onClick: node => { console.log('Clicked node: ', node) },
    },
  }
  

const cosmograph = new Cosmograph(canvas)
cosmograph.setConfig(config)

cosmograph.setData(nodes, links)