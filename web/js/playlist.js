import { findUserById, apiPlaylistMove, apiPlaylistRemove } from "./main.js"

export { Playlist }

class Playlist {
    constructor() {
        this.htmlEntries = document.getElementById("playlist_entries");
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

    add(entry) {
        this.entries.push(entry);
        let element = this.createHtmlEntry(entry, this.entries.length);
        this.htmlEntries.appendChild(element);
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
            apiPlaylistMove(this.entries[startIndex].id, startIndex, endIndex);
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
            apiPlaylistRemove(this.entries[index].id, index);
        };
        button.textContent = "Remove";
        buttonCell.appendChild(button);

        return htmlEntry;
    }

    removeFirst() {
        if (this.entries.length >= 0) {
            return this.removeAt(0);
        } 

        return null;
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

        return entry;
    }

    clear() {
        this.entries = [];
        this.htmlEntries

        let container = this.htmlEntries;
        while (container.firstChild) {
            container.removeChild(container.lastChild);
        }
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
}
