import cytoscape from 'cytoscape';
import jsonData from "./nomad-json.json";

const data: any = jsonData;

let nodes: any[] = [];
let edges: any[] = [];

// data.findings.forEach((finding: any) => {
//     finding.Paths.forEach((paths: string[]) => {
//         let prev = "";
//         paths.forEach((path, i) => {
//             if (i === 0) {
//                 prev = path;
//                 addBaseNode(path);
//             } else {
//                 addNode(path);
//                 addEdge(path, prev);
//                 prev = path;
//             }
//         });
//     });
// });

// if (data.findings != undefined) {
    data.findings.forEach((finding: any) => {
        // console.log(finding)
        finding.Paths.forEach((paths: string | any[]) => {
            let prev = "";
            for (let i = 0; i < paths.length; i++) {
                if (i == 0) {
                    prev = paths[i]
                    addBaseNode(paths[i])
                } else {
                    addNode(paths[i])
                    addEdge(paths[i], prev)
                    prev = paths[i]  
                }  
            }
        })
    });
// }

function addNode(node: string) {
    nodes.push({data: { id: node }, style: {'background-color': 'red', 'label': node }});
    // graph.addNode(node, {label: node });
}

function addBaseNode(node: string) {
    nodes.push({data: { id: node }, style: {'background-color': 'blue', 'label': node}});
    // graph.addNode(node, {label: node });
}

function addEdge(nodeFrom: string, nodeTo: string) {
    edges.push({data: {id: nodeFrom + "->" + nodeTo, source: nodeFrom, target: nodeTo}})
    // if (!graph.hasEdge(nodeFrom, nodeTo)) {
    //     graph.addEdge(nodeFrom, nodeTo);
    // }
}



var cy = cytoscape({
    container: document.getElementById('sigma-container'), // container to render in

    elements: { // list of graph elements to start with
        nodes: nodes,
        edges: edges
    },

    boxSelectionEnabled: false,
  
    style: cytoscape.stylesheet()
    //   .selector('node')
    //     .style({
    //       'content': 'data(id)'
    //     })
      .selector('edge')
        .style({
          'curve-style': 'bezier',
          'target-arrow-shape': 'triangle',
          'width': 4,
          'line-color': '#ddd',
          'target-arrow-color': '#ddd'
        }),
    //   .selector('.highlighted')
    //     .style({
    //       'background-color': '#61bffc',
    //       'line-color': '#61bffc',
    //       'target-arrow-color': '#61bffc',
    //       'transition-property': 'background-color, line-color, target-arrow-color',
    //       'transition-duration': '0.5s'
    //     }),

  layout: {
    name: 'breadthfirst',
    directed: true,
    padding: 10,
    fit: true,
    spacingFactor: 1.0,
    nodeDimensionsIncludeLabels: true
  }
});


// cy.nodes('[id="N2"]').style('background-color', 'red');

// // if (data.findings != undefined) {
//     // data.findings.forEach((finding: any) => {
//     //     // console.log(finding)
//     //     finding.Paths.forEach((paths: string | any[]) => {
//     //         let prev = "";
//     //         for (let i = 0; i < paths.length; i++) {
//     //             if (i == 0) {
//     //                 prev = paths[i]
//     //                 addNode(paths[i])
//     //             } else {
//     //                 addNode(paths[i])
//     //                 addEdge(paths[i], prev)
//     //                 prev = paths[i]  
//     //             }  
//     //         }
//     //     })
//     // });
// // }

// import Sigma from "sigma";
// import Graph from "graphology";
// import circular from "graphology-layout/circular";
// import forceAtlas2 from "graphology-layout-forceatlas2";
// import { Coordinates, NodeDisplayData, EdgeDisplayData } from "sigma/types";

// import jsonData from "./nomad-json.json";

// const data: any = jsonData;

// // Retrieve some useful DOM elements:
// const container = document.getElementById("sigma-container") as HTMLElement;
// const searchInput = document.getElementById("search-input") as HTMLInputElement;
// const searchSuggestions = document.getElementById("suggestions") as HTMLDataListElement;

// // Instantiate sigma:
// const graph = new Graph();

// data.findings.forEach((finding: any) => {
//     finding.Paths.forEach((paths: string[]) => {
//         let prev = "";
//         paths.forEach((path, i) => {
//             if (i === 0) {
//                 prev = path;
//                 addNode(path);
//             } else {
//                 addNode(path);
//                 addEdge(path, prev);
//                 prev = path;
//             }
//         });
//     });
// });

// function addNode(node: string) {
//     if (!graph.hasNode(node)) {
//         graph.addNode(node, {label: node });
//     }
// }

// function addEdge(nodeFrom: string, nodeTo: string) {
//     if (!graph.hasEdge(nodeFrom, nodeTo)) {
//         graph.addEdge(nodeFrom, nodeTo);
//     }
// }

// circular.assign(graph);
// const settings = forceAtlas2.inferSettings(graph);
// forceAtlas2.assign(graph, { settings, iterations: 600 });


// // Type and declare internal state:
// interface State {
//   hoveredNode?: string;
//   searchQuery: string;
//   clickedNode?: string;
//   connectedEdges?: Set<string>;

//   // State derived from query:
//   selectedNode?: string;
//   suggestions?: Set<string>;

//   // State derived from hovered node:
//   hoveredNeighbors?: Set<string>;
// }

// const state: State = { searchQuery: "" };

// // Feed the datalist autocomplete values:
// searchSuggestions.innerHTML = graph
//   .nodes()
//   .map((node) => `<option value="${graph.getNodeAttribute(node, "label")}"></option>`)
//   .join("\n");

// const renderer = new Sigma(graph, container);

// renderer.on("clickNode", ({ node }) => {
//     console.log("called")
//     // setClickedNode(node);
// });


// // Actions:
// function setSearchQuery(query: string) {
//   state.searchQuery = query;

//   if (searchInput.value !== query) searchInput.value = query;

//   if (query) {
//     const lcQuery = query.toLowerCase();
//     const suggestions = graph
//       .nodes()
//       .map((n) => ({ id: n, label: graph.getNodeAttribute(n, "label") as string }))
//       .filter(({ label }) => label.toLowerCase().includes(lcQuery));

//     if (suggestions.length === 1 && suggestions[0].label === query) {
//       state.selectedNode = suggestions[0].id;
//       state.suggestions = undefined;

//       const nodePosition = renderer.getNodeDisplayData(state.selectedNode) as Coordinates;
//       renderer.getCamera().animate(nodePosition, {
//         duration: 500,
//       });
//     } else {
//       state.selectedNode = undefined;
//       state.suggestions = new Set(suggestions.map(({ id }) => id));
//     }
//   } else {
//     state.selectedNode = undefined;
//     state.suggestions = undefined;
//   }

//   renderer.refresh();
// }

// function setHoveredNode(node?: string) {
//   if (node) {
//     state.hoveredNode = node;
//     state.hoveredNeighbors = new Set(graph.neighbors(node));
//   } else {
//     state.hoveredNode = undefined;
//     state.hoveredNeighbors = undefined;
//   }

//   renderer.refresh();
// }

// function setClickedNode(node?: string) {
//   if (node) {
//     state.clickedNode = node;
//     const connectedEdges = graph.edges(node);
//     state.connectedEdges = new Set(connectedEdges);
//   } else {
//     state.clickedNode = undefined;
//     state.connectedEdges = undefined;
//   }

//   renderer.refresh();
// }

// // Bind search input and graph interactions:
// searchInput.addEventListener("input", () => {
//   setSearchQuery(searchInput.value || "");
// });
// searchInput.addEventListener("blur", () => {
//   setSearchQuery("");
// });

// renderer.on("enterNode", ({ node }) => {
//   setHoveredNode(node);
// });
// renderer.on("leaveNode", () => {
//   setHoveredNode(undefined);
// });
// renderer.on("clickNode", ({ node }) => {
//     console.log("called")
//   setClickedNode(node);
// });

// // Render nodes and edges accordingly to the internal state:
// renderer.setSetting("nodeReducer", (node, data) => {
//   const res: Partial<NodeDisplayData> = { ...data };

//   if (state.hoveredNode) {
//     if (state.hoveredNode !== node && !state.hoveredNeighbors?.has(node)) {
//       res.label = "";
//       res.color = "#f6f6f6";
//     }
//   }

//   if (state.clickedNode) {
//     if (state.clickedNode === node || (state.connectedEdges && Array.from(state.connectedEdges).some(edge => graph.hasExtremity(edge, node)))) {
//       res.color = "#ff0000";
//     } else {
//       res.label = "";
//       res.color = "#f6f6f6";
//     }
//   }

//   if (state.selectedNode === node) {
//     res.highlighted = true;
//   } else if (state.suggestions && !state.suggestions.has(node)) {
//     res.label = "";
//     res.color = "#f6f6f6";
//   }

//   return res;
// });

// renderer.setSetting("edgeReducer", (edge, data) => {
//   const res: Partial<EdgeDisplayData> = { ...data };

//   if (state.hoveredNode && !graph.hasExtremity(edge, state.hoveredNode)) {
//     res.hidden = true;
//   }

//   if (state.clickedNode && (!state.connectedEdges?.has(edge))) {
//     res.hidden = true;
//   }

//   if (state.suggestions && (!state.suggestions.has(graph.source(edge)) || !state.suggestions.has(graph.target(edge)))) {
//     res.hidden = true;
//   }

//   return res;
// });
