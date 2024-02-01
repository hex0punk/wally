import { Cosmograph,  CosmographSearch } from '@cosmograph/cosmograph'
import jsonData from "../nomad-json-3.json";

const details = document.getElementById('details');

// Types
type Node = {
    id: string;
    color: string;
    green: string;
    label: string;
    finding: string;
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
                addNodeIfNotExist(path, "purple", "");
            } else {
                if (i == paths.length - 1) {
                    addNodeIfNotExist(path, "#984040", finding.MatchId);
                } else {
                    addNodeIfNotExist(path, "#4287f5", "");
                }
                addEdgeIfNotExist(prev, path);
                prev = path;
            }
        });
    });
});

function getFinding(findingId: string): any {
    // console.log("looking for", findingId)
    let res : any;
    data.forEach((finding: any) => {
        if (findingId == finding.MatchId) {
            res = finding
        }
    })
    return res
}

function nodeExists(nodeId: string): boolean {
    return nodes.some(node => node.id === nodeId);
}

function edgeExists(source: string, target: string): boolean {
    return links.some(link => link.source === source && link.target === target);
}

function addNodeIfNotExist(nodeId: string, color: string, findingId: string) {
    if (!nodeExists(nodeId)) {
        let label = extractFuncFromId(nodeId)
        label = label != null ? label : ""
        nodes.push({ id: nodeId, label: label, color: color, green: "green", finding: findingId });
    } else {
        let node = nodes.find(node => node.id === nodeId)
        if (node != null) {
            if (color == "#4287f5" && node.color == "purple") {
                node.color = "#FFCE85"
            }

            if (node.color == "#4287f5" && color == "purple") {
                node.color = "#FFCE85"
            }
        }
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
    const defaultColor = node.color
    const clickedColor = 'green'; // Red
  
    if (clickedNodeId == node.id) {
    // Check if the current node is the one that was clicked
    // if (clickedNodeIdList.includes(node.id)) {
      return clickedColor;
    } else {
      return defaultColor;
    }
}

function getClickedNodesColor(node: Node) {
    const defaultColor = node.color
    const clickedColor = 'green'; // Red
  
    // if (clickedNodeId == node.id) {
    // Check if the current node is the one that was clicked
    if (clickedNodes.includes(node.id)) {
        return node.color
    } else {
        if (clickedNodes.length > 0) {
            return [0, 0, 0, 0]
        } else {
            if (node.color == "purple" || node.color == "#984040") {
                return node.color
            }
            return defaultColor;
        }
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
    if (clickedNodes.length > 0) {
        return [0, 0, 0, 0]
    } 
    return link.color
}

function getLabel(node: Node) {
    if (clickedNodes.includes(node.id)) {
        return node.id
    } else {
        if (clickedNodes.length > 0) {
            return ''
        } else {
            return node.label;
        }
    }
}

let config = {
    backgroundColor: "#0f172a",
    nodeSize: 2.0,
    nodeColor: n => getClickedNodesColor(n),
    // linkWidth: 0.5,
    linkColor: (l) => getLinkColor(l),
    // linkArrows: true,
    // linkVisibilityDistance: [100, 150],
    nodeLabelAccessor: n => getLabel(n),
    nodeLabelColor: 'white',
    simulationRepulsion: 1.6,
    simulationLinkDistance: 10,
    nodeLabelClassName: "css-label--label",
    // renderLinks: true,
    onClick: (node, i) => { 
        // console.log(node)
        if (node == undefined) {
            clickedNodes = []
            clickedNodeId = ""
            detailsOff()
        } else {
            let conn = findAllPrecedingNodes(node.id)
            clickedNodes = conn
            clickedNodes.push(node.id)
            clickedNodeId = node.id

            console.log(JSON.stringify(clickedNodes))
            if (node.finding != "") {
                console.log("here")
                let finding = getFinding(node.finding)
                setLeftSide(finding)
                // let out = document.getElementById("output")
                // // console.log("got finding ", finding)
                // if (out != null) {
                //     console.log("setting out")
                //     out.textContent = finding
                // }
            }
        }

        config.nodeColor = (n) => getClickedNodesColor(n)
        config.linkColor = (l) => getLinkColor(l)
        cosmograph.setConfig(config)
    },
    // disableSimulation: true
  }
  
function detailsOn() {
    if (details != null && details.classList.contains('invisible')) {
        details.classList.remove('invisible');
    }

}

function detailsOff() {
    if (details != null && !details.classList.contains('invisible')) {
        details.classList.add('invisible');
    }
}

function setLeftSide(finding: any) {
    // console.log(finding)
    detailsOn()
    document.getElementById('pkg').textContent = finding.Indicator.Package
    document.getElementById('func').textContent = finding.Indicator.Function
    document.getElementById('params').textContent = JSON.stringify(finding.Indicator.Params)
    document.getElementById('enclosedBy').textContent = finding.EnclosedBy
    document.getElementById('pos').textContent = finding.Pos
    document.getElementById('pathNum').textContent = finding.Paths.length
}

const searchContainer = document.getElementById("cosmosearch")

const search = new CosmographSearch<Node, Link>(cosmograph,searchContainer)

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
