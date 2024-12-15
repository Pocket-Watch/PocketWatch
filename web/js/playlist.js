import * as api from "./api.js";

export { Playlist }

class Playlist {
    constructor() {
        this.htmlEntryList = document.getElementById("playlist_entry_list");
        // NOTE: Other items go here like control buttons and input boxes.

        /// Corresponds to the actual playlist entries on the server.
        this.entries = [];

        /// Corresponds to html elements created from the playlist entries.
        this.htmlEntries = [];
    }

    loadEntries(entries) {
        if (!entries) {
            console.warn("WARN: Failed to load entries, function input argument is null.");
            return;
        }

        this.entries = entries;
        for (let i = 0; i < this.entries.length; i++) {
            const entry = this.entries[i];
            const htmlEntry = this.createHtmlEntry(entry);
            this.htmlEntries.push(htmlEntry);
        }
    }

    createHtmlEntry(entry) {

    }
}
