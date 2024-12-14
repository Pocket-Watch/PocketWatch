import * as api from "./api.js";

export { Playlist }

class Playlist {
    constructor() {
        this.htmlEntryList = document.getElementById("playlist_entry_list");
        // NOTE: Other items go here like control buttons and input boxes;

        /// Corresponds to actual playlist entries on the server.
        this.entires     = [];

        /// Corresponds to html elements created from the playlist entries.
        this.htmlEntries = [];
    }

    loadEntries(entries) {
        if (!entries) {
            console.info("INFO: Loaded 0 entires. Playlist is empty");
            return;
        }
    }
}
