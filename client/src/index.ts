import { Cosmograph,  CosmographSearch } from '@cosmograph/cosmograph'
import jsonData from "../nomad-json-2.json";

// Types
type Node = {
    id: string;
    color: string;
    green: string;
    label: string;
    x?: number;
    y?: number;
  }
  
  type Link = {
    source: string;
    target: string;
    color: string;
  }


// Load data from json
const data: any = jsonData;

let nodes: Node[] = [];
let links: Link[] = [];
let clickedNodes: string[] = [];
let clickedNodeId: string;

data.forEach((finding: any) => {
    finding.Paths.forEach((paths: string[]) => {
        let prev = "";
        paths.forEach((path, i) => {
            if (i === 0) {
                prev = path;
                addNodeIfNotExist(path, "#984040");
            } else {
                if (i == paths.length - 1) {
                    addNodeIfNotExist(path, "purple");
                } else {
                    addNodeIfNotExist(path, "#4287f5");
                }
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
        nodes.push({ id: nodeId, label: label, color: color, green: "green" });
    }
}

function addEdgeIfNotExist(source: string, target: string) {
    if (!edgeExists(source, target)) {
        links.push({ source, target, color: "#8C8C8C" });
    }
}

function extractFuncFromId(nodeId: string): string | null {
    const match = nodeId.match(/\[(.*?)\]/);
    return match ? match[1] : null;
}

function findLinksByNodeId(nodeId: string): Link[] {
    return links.filter(link => link.source === nodeId || link.target === nodeId);
}

function getClickedNodeColor(node: Node) {
    // Define the default color and the color for a clicked node
    console.log("called")
    const defaultColor = node.color
    const clickedColor = 'green'; // Red
  
    if (clickedNodeId == node.id) {
        console.log("yep")
    // Check if the current node is the one that was clicked
    // if (clickedNodeIdList.includes(node.id)) {
      return clickedColor;
    } else {
      return defaultColor;
    }
}

function getClickedNodesColor(node: Node) {
    // Define the default color and the color for a clicked node
    if (node.color == "purple" || node.color == "#984040") {
        return node.color
    }
    console.log("called")
    const defaultColor = node.color
    const clickedColor = 'green'; // Red
  
    // if (clickedNodeId == node.id) {
        console.log("yep")
    // Check if the current node is the one that was clicked
    if (clickedNodes.includes(node.id)) {
      return clickedColor;
    } else {
      return defaultColor;
    }
}


function findAllConnectedNodes(nodeId: string): string[] {
    let visited = new Set(); // To keep track of visited nodes
    let stack = [nodeId]; // Use a stack for depth-first search

    while (stack.length > 0) {
        let current = stack.pop();

        // Add the current node to the visited set
        visited.add(current);

        // Find all links where the current node is a source or target
        let connectedLinks = findLinksByNodeId(current);
        connectedLinks.forEach(link => {
            // Check both the source and target of each link
            if (!visited.has(link.source)) {
                stack.push(link.source);
            }
            if (!visited.has(link.target)) {
                stack.push(link.target);
            }
        });
    }

    // Convert the Set of visited nodes to an Array and return
    return Array.from(visited);
}

// function findLinksByNodeId(nodeId: string): Link[] {
//     return links.filter(link => link.source === nodeId || link.target === nodeId);
// }


function findAllPrecedingNodes(nodeId: string): string[] {
    let visited = new Set(); // To keep track of visited nodes
    let stack = [nodeId]; // Start with the target node

    while (stack.length > 0) {
        let current = stack.pop();

        // Add the current node to the visited set
        visited.add(current);

        // Find all links where the current node is a target
        let incomingLinks = links.filter(link => link.target === current);
        incomingLinks.forEach(link => {
            // Add the source node of each link to the stack
            if (!visited.has(link.source)) {
                stack.push(link.source);
            }
        });
    }

    visited.delete(nodeId); // Remove the initial node from the result
    return Array.from(visited); // Convert the Set of visited nodes to an Array and return
}

// const canvas = document.getElementById("container")

const cosmographContainer = document.getElementById("cosmograph")
const cosmograph = new Cosmograph<Node, Link>(cosmographContainer)

// Now set the color of the link
// If the target is selected, then the link should be green
function getLinkColor(link: Link) {
    let nt = clickedNodes.find(n => link.target === n)
    if (nt != undefined && nt != null) {
        return "green";
    }
    return link.color
}

let config = {
    backgroundColor: "#0f172a",
    nodeSize: 2.0,
    nodeColor: n => getClickedNodesColor(n),
    // linkWidth: 0.5,
    linkColor: (l) => getLinkColor(l),
    // linkArrows: true,
    // linkVisibilityDistance: [100, 150],
    nodeLabelAccessor: n => n.label,
    nodeLabelColor: 'white',
    simulationRepulsion: 1.6,
    simulationLinkDistance: 10,
    nodeLabelClassName: "css-label--label",
    // renderLinks: true,
    onClick: (node, i) => { 
        if (node == undefined) {
            clickedNodes = []
            clickedNodeId = ""
        } else {
            let conn = findAllPrecedingNodes(node.id)
            clickedNodes = conn
            clickedNodes.push(node.id)
            clickedNodeId = node.id
        }

        config.nodeColor = (n) => getClickedNodesColor(n)
        config.linkColor = (l) => getLinkColor(l)
        cosmograph.setConfig(config)
    },
    // disableSimulation: true
  }
  


const searchContainer = document.getElementById("cosmosearch")

const search = new CosmographSearch<Node, Link>(cosmograph, searchContainer)

cosmograph.setConfig(config)

const searchConfig = {
    maxVisibleItems: 5,
    events: {
      onSelect: (node) => {
            console.log('Selected Node: ', node.id)
        }
    }
  }
    
search.setConfig(searchConfig)

cosmograph.setData(nodes, links)
// search.setData(nodes)
