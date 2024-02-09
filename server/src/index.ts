import { Node, Link } from "./cosmograph/types";
import { WallyGraph } from "./cosmograph/graph";

// Function to fetch and parse wally data
async function loadWallyData() {
    const fileName = "wally.json"
    try {
        const fileUrl = `/${fileName}`;
        
        // Fetch wally file hosted by Go
        const response = await fetch(fileUrl);
        if (!response.ok) {
            throw new Error(`Failed to fetch ${fileUrl}: ${response.statusText}`);
        }

        const jsonData = await response.json();
        const wallyGraph = new WallyGraph(jsonData);
        wallyGraph.setupGraph();     
    } catch (error) {
        console.error('Error loading Wally data:', error);
    }
}

loadWallyData()
