import { findUserById, playerArea } from "./main_old.js"
import * as api from "./api_old.js";

export { Playlist }

class Playlist {
    constructor() {
        this.htmlRoot = document.getElementById("playlist_root");
        this.htmlUrlInput = document.getElementById("playlist_url_input");
        this.htmlEntries = document.getElementById("playlist_entries");
        this.htmlEntryCount = document.getElementById("playlist_entry_count");

        this.entries = [];

        this.dragEntryStart = null;
        this.dragEntryEnd = null;
    }

    findHtmlElementIndex(target) {
        let element = this.htmlEntries.firstChild;
        if (!element) {
            return -1;
        }

        let index = 0;
        while (element) {
            if (element === target) {
                return index;
            }

            element = element.nextSibling;
            index += 1;
        }

        return index;
    }

    updateHtmlEntryCount() {
        this.htmlEntryCount.textContent = this.entries.length;
    }

    add(entry) {
        this.entries.push(entry);
        let element = this.createHtmlEntry(entry, this.entries.length);
        this.htmlEntries.appendChild(element);
        this.updateHtmlEntryCount();
    }

    createHtmlEntry(entry, position) {
        let htmlEntry = document.createElement("tr");
        htmlEntry.draggable = true;

        htmlEntry.ondragstart = (event) => { 
            event.dataTransfer.effectAllowed = "move";
            this.dragEntryStart = event.target;
            this.dragEntryEnd = event.target;
        };

        htmlEntry.ondragover = (event) => { 
            let element = event.target.parentNode;
            if (this.dragEntryEnd === element) {
                return;
            }

            this.dragEntryEnd = element;
        };

        htmlEntry.ondragend = (_event) => { 
            if (this.dragEntryStart === this.dragEntryEnd) {
                return;
            }

            let startIndex = this.findHtmlElementIndex(this.dragEntryStart);
            let endIndex = this.findHtmlElementIndex(this.dragEntryEnd);
            api.playlistMove(this.entries[startIndex].id, startIndex, endIndex);
        };

        let positionTh = document.createElement("th");
        positionTh.textContent = position + ".";
        positionTh.scope = "row";
        htmlEntry.appendChild(positionTh);

        // NOTE(kihau): This function in exported from main.js. Doing it this way is a little bit werid.
        let user = findUserById(entry.user_id);
        let username = user.username;
        if (!username) {
            username = "<unknown>";
        }
        let usernameCell = htmlEntry.insertCell(-1);
        usernameCell.textContent = username;

        let titleCell = htmlEntry.insertCell(-1);
        if (entry.title !== "") {
            let a = document.createElement("a");
            a.href = entry.url;
            a.textContent  = entry.title;
            titleCell.appendChild(a);
        } else {
            titleCell.textContent = entry.url;
        }

        let buttonCell = htmlEntry.insertCell(-1);
        let button = document.createElement("button");
        button.onclick = (event) => {
            let entry = event.target.parentElement.parentElement;
            let index = this.findHtmlElementIndex(entry);
            api.playlistRemove(this.entries[index].id, index);
        };
        button.textContent = "Remove";
        buttonCell.appendChild(button);

        return htmlEntry;
    }

    removeFirst() {
        if (this.entries.length === 0) {
            console.warn("WARN: Playlist::removeFirst called but entries length is 0");
            return null;
        } 

        return this.removeAt(0);
    }

    removeAt(index) {
        if (index < 0 || index >= this.entries.length) {
            console.error("ERROR: Cannot remove playlist entry, index " + index + " is out of bounds.");
            return null;
        }

        let entry = this.entries[index];
        this.entries.splice(index, 1);

        let table = this.htmlEntries.rows;
        table[index].parentNode.removeChild(table[index]);
        for (var i = 0; i < table.length; i++) {
            table[i].getElementsByTagName("th")[0].textContent = i + 1 + ".";
        }

        this.updateHtmlEntryCount();
        return entry;
    }

    clear() {
        this.entries = [];
        this.htmlEntries

        let container = this.htmlEntries;
        while (container.firstChild) {
            container.removeChild(container.lastChild);
        }

        this.updateHtmlEntryCount();
    }

    loadNew(entries) {
        this.clear();
        if (!entries) {
            return;
        }

        for (var i = 0; i < entries.length; i++) {
            let element = this.createHtmlEntry(entries[i], i + 1);
            this.htmlEntries.appendChild(element);
        }

        this.entries = entries;
        this.updateHtmlEntryCount();
    }

    move(sourceIndex, destIndex) {
        let entry = this.removeAt(sourceIndex);
        if (!entry) {
            console.warn("WARN: Playlist::move failed to move element at:", sourceIndex, "to index:", destIndex);
            return;
        }

        this.entries.splice(destIndex, 0, entry);
        this.loadNew(this.entries);
    }

    updateUsernames(updatedUser) {
        let table = this.htmlEntries.rows;
        for (var i = 0; i < this.entries.length; i++) {
            let entry = this.entries[i];
            if (entry.user_id == updatedUser.id) {
                table[i].getElementsByTagName("td")[0].textContent = updatedUser.username;
            }
        }
    }

    requestEntryAdd() {
        if (!this.htmlUrlInput.value) {
            console.warn("WARNING: Url is empty, not adding to the playlist.");
            return;
        }

        // NOTE(kihau): This creates entry based on input options below the player, which is werid.
        let entry = playerArea.createApiEntry(this.htmlUrlInput.value);
        api.playlistAdd(entry);

        // NOTE(kihau): Do not clear on request failed?
        this.htmlUrlInput.value = "";
    }

    attachHtmlEventHandlers() {
        let data = this;

        window.inputPlaylistOnKeypress = (event) => {
            if (event.key === "Enter") {
                data.requestEntryAdd();
            }
        };

        window.playlistAddOnClick = () => {
            data.requestEntryAdd();
        };

        window.playlistShuffleOnClick = api.playlistShuffle;
        window.playlistClearOnClick = api.playlistClear;
    }

    handleServerEvent(action, data) {
        switch (action) {
            case "add": {
                this.add(data);
            } break;

            case "clear": {
                this.clear()
            } break;

            case "remove": {
                this.removeAt(data)
            } break;

            case "shuffle": {
                this.loadNew(data);
            } break;

            case "move": {
                this.move(data.source_index, data.dest_index);
            } break;

            default: {
                console.error("Unknown playlist action:", action, "with data:", data);
            } break;
        }
    }
}
