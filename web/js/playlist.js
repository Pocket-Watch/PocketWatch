import * as api from "./api.js";

export { Playlist }

function div(className) {
    let element = document.createElement("div");
    element.className = className;
    return element;
}

function a(className, textContent, href) {
    let element = document.createElement("a");
    element.className = className;
    element.textContent = textContent;
    element.href = href;
    return element;
}

function img(src) {
    let element = document.createElement("img");
    element.src = src;
    return element;
}

function svg(href) {
    let svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    let use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", href);
    svg.appendChild(use);
    return svg;
}

function button(className, title) {
    let element = document.createElement("button");
    element.className = className;
    element.title = title;
    return element;
}

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
            this.htmlEntryList.appendChild(htmlEntry);
        }
    }

    createHtmlEntry(entry) {
        let entryDiv       = div("playlist_entry");
        let entryTop       = div("playlist_entry_top"); 
        let entryDragArea  = div("playlist_drag_area"); 
        let entryThumbnail = div("playlist_entry_thumbnail");
        // NOTE(kihau): 
        //     This will be an actual thumbnail image. 
        //     The placeholder will be set when the entry thumbnail image is an empty string.
        let thumbnailImg   = img("img/thumbnail_placeholder.png");
        let entryInfo      = div("playlist_entry_info");
        let entryTitle     = div("playlist_entry_title");
        let entryUrl       = a("playlist_entry_url", entry.url, entry.url);
        let entryButtons   = div("playlist_entry_buttons");
        let editButton     = button("playlist_entry_edit_button", "Edit playlist entry")
        let editSvg        = svg("svg/main_icons.svg#edit");
        let deleteButton   = button("playlist_entry_delete_button", "Delete playlist entry")
        let deleteSvg      = svg("svg/main_icons.svg#delete");
        let dropdownButton = div("playlist_dropdown_button");
        let entryDropdown  = div("playlist_entry_dropdown"); 

        entryDragArea.textContent = "☰";
        entryTitle.textContent = entry.title;
        dropdownButton.textContent = "▼";

        editButton.onclick = () => {
            console.log("edit button clicked");
        };

        deleteButton.onclick = () => {
            console.log("delete button clicked");
        };

        dropdownButton.onclick = () => {
            entryDiv.classList.toggle("entry_dropdown_expand");
        };

        entryDiv.append(entryTop); {
            entryTop.append(entryDragArea);
            entryTop.append(entryThumbnail); {
                entryThumbnail.append(thumbnailImg);
            }
            entryTop.append(entryInfo); {
                entryInfo.append(entryTitle);
                entryInfo.append(entryUrl);
            }
            entryTop.append(entryButtons); {
                entryButtons.append(editButton); {
                    editButton.append(editSvg);
                }
                entryButtons.append(deleteButton); {
                    deleteButton.append(deleteSvg);
                }
            }
            entryTop.append(dropdownButton); {

            }
        }
        entryDiv.append(entryDropdown);

        return entryDiv;
    }
}
