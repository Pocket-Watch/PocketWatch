import * as api from "./api.js";
import { getById, div, a, span, img, svg, button } from "./util.js";

export { Playlist }

const ENTRY_ROW_GAP = 4;
const ENTRY_BORDER  = 2;
const ENTRY_HEIGHT  = 64 + ENTRY_BORDER * 2;

class Playlist {
    constructor() {
        this.htmlEntryListWrap = getById("playlist_entry_list_wrap");
        this.htmlEntryList     = getById("playlist_entry_list");
        this.controlsRoot      = getById("playlist_controls_root");
        this.dropdownButton    = getById("playlist_dropdown_button");

        this.draggableEntry    = null;
        this.draggableTopStart = 0.0;

        /// Only one entry is allowed to be expanded at a time.
        this.expandedEntry = null;

        /// Corresponds to the actual playlist entries on the server.
        this.entries = [];

        /// old: Corresponds to html elements created from the playlist entries. 
        /// Represents the structure of the htmlEntryList post transition while entries are still mid transition.
        this.htmlEntries = [];

        this.dragStartIndex   = -1;
        this.dragCurrentIndex = -1;
    }

    attachPlaylistEvents() {
        this.dropdownButton.onclick = _ => {
            console.log("clicked");
            this.controlsRoot.classList.toggle("playlist_controls_root_expand");
        };
    }

    addEntry(entry) {
        this.entries.push(entry);

        const htmlEntry = this.createHtmlEntry(entry);
        this.setEntryPosition(htmlEntry, this.htmlEntries.length);

        this.htmlEntries.push(htmlEntry);
        this.htmlEntryList.appendChild(htmlEntry);
    }

    // TODO(kihau): Proper networking handling.
    removeAt(index) {
        if (typeof index !== "number") {
            console.error("ERROR: Playlist::removeAt failed. The input index:", index, "is invalid.");
            return;
        }

        if (index < 0 || index >= this.entries.length) {
            console.error("ERROR: Playlist::removeAt failed. Index:", index, "is out of bounds for array:", this.entries);
            return;
        }

        let entry = this.entries[index];
        this.entries.splice(index, 1);

        let htmlEntry = this.htmlEntries[index];
        this.htmlEntries.splice(index, 1);

        this.htmlEntryList.removeChild(htmlEntry);
        for (let i = index; i < this.htmlEntries.length; i++) {
            const entry = this.htmlEntries[i];
            this.setEntryPosition(entry, i);
        }
    }

    // TODO(kihau): Proper networking handling.
    move(sourceIndex, destIndex) {
        console.log(sourceIndex, destIndex);

        if (sourceIndex === destIndex) {
            return;
        } else if (destIndex < sourceIndex) {
            for (let i = destIndex; i < sourceIndex; i++) {
                this.setEntryPosition(this.htmlEntries[i], i + 1);
            }
        } else if (destIndex > sourceIndex) {
            for (let i = destIndex; i > sourceIndex; i--) {
                this.setEntryPosition(this.htmlEntries[i], i - 1);
            }
        }

        let htmlEntry = this.htmlEntries[sourceIndex];
        this.htmlEntries.splice(sourceIndex, 1);
        this.htmlEntries.splice(destIndex, 0, htmlEntry);

        let entry = this.entries[sourceIndex];
        this.entries.splice(sourceIndex, 1);
        this.entries.splice(destIndex, 0, entry);

        this.setEntryPosition(htmlEntry, destIndex);
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
            this.setEntryPosition(htmlEntry, this.htmlEntries.length);

            this.htmlEntries.push(htmlEntry);
            this.htmlEntryList.appendChild(htmlEntry);
        }
    }

    clear() {
        while (this.htmlEntryList.lastChild) {
            this.htmlEntryList.removeChild(this.htmlEntryList.lastChild);
        }

        this.htmlEntries    = [];
        this.entries       = [];
        this.expandedEntry = null;

        this.dragStartIndex    = -1;
        this.dragCurrentIndex  = -1;
        this.draggableEntry    = null;
        this.draggableTopStart = 0.0;
    }

    findEntryIndex(entry) {
        return this.entries.findIndex(item => item === entry);
    }

    findHtmlIndex(htmlEntry) {
        return this.htmlEntries.findIndex(item => item === htmlEntry);
    }

    expandEntry(entry) {
        if (this.expandedEntry) {
            this.expandedEntry.classList.remove("entry_dropdown_expand");
        }

        if (entry) {
            this.expandedEntry = entry;
            this.expandedEntry.classList.add("entry_dropdown_expand");
        }
    }

    collapseEntry(entry) {
        if (entry !== this.expandedEntry) {
            return;
        }

        // NOTE(kihau): To reduce visible DOM size we could apply "display: none" after the transition.
        if (this.expandedEntry) {
            this.expandedEntry.classList.remove("entry_dropdown_expand");
        }

        this.expandedEntry = null;
    }

    indexToPosition(index) {
        const rect = this.htmlEntryList.getBoundingClientRect();
        let positionY = rect.y + (ENTRY_HEIGHT + ENTRY_ROW_GAP) * index;
        return positionY;
    }

    positionToIndex(positionY) {
        const rect = this.htmlEntryList.getBoundingClientRect();
        let index = Math.floor((positionY - rect.y) / (ENTRY_HEIGHT + ENTRY_ROW_GAP));

        if (index < 0) {
            index = 0;
        }

        if (index >= this.htmlEntries.length) {
            index = this.htmlEntries.length - 1;
        }

        return index;
    }

    getTranslateY(element) {
        let y = 0.0;
        if (element.style.transform) {
            let string = element.style.transform;

            let start = 0;
            let end   = string.length;

            for (let i = 0; i < string.length; i++) {
                if (string.charAt(i) === "(") {
                    start = i + 1;
                }

                if (string.charAt(i) === "p") {
                    end = i;
                }
            }

            y = Number(string.substring(start, end));
            console.warn(string, y);
        }

        return y;
    }

    setEntryPosition(entry, index) {
        entry.style.top = index * (ENTRY_HEIGHT + ENTRY_ROW_GAP) + "px";
    }

    createHtmlEntry(entry) {
        let entryRoot      = div("playlist_entry");
        let entryTop       = div("playlist_entry_top"); 
        let entryDragArea  = div("playlist_drag_area"); 
        let entryThumbnail = div("playlist_entry_thumbnail");
        let thumbnailImg   = img("img/thumbnail_placeholder.png");
        let entryInfo      = div("playlist_entry_info");
        let entryTitle     = span("playlist_entry_title", entry.title);
        let entryUrl       = a("playlist_entry_url", entry.url);
        let entryButtons   = div("playlist_entry_buttons");
        let editButton     = button("playlist_entry_edit_button", "Edit playlist entry")
        let editSvg        = svg("svg/main_icons.svg#edit");
        let deleteButton   = button("playlist_entry_delete_button", "Delete playlist entry")
        let deleteSvg      = svg("svg/main_icons.svg#delete");
        let dropdownButton = div("playlist_dropdown_button");
        let dropdownSvg    = svg("svg/main_icons.svg#dropdown");
        let entryDropdown  = div("playlist_entry_dropdown"); 
        let dropdownTop    = div("playlist_dropdown_info_top"); 
        let addedByText    = span("playlist_dropdown_added_by", "Added by"); 
        let createdAtText  = span("playlist_dropdown_created_at", "Created at"); 
        let dropdownBottom = div("playlist_dropdown_info_bottom"); 
        let userAvatar     = div("playlist_dropdown_user_avatar");
        let userAvatarImg  = img("img/default_avatar.png");
        let userName       = span("playlist_dropdown_user_name", "Placeholder " + entry.user_id);
        let date           = new Date(entry.created);
        let creationDate   = span("playlist_dropdown_creation_date", date.toLocaleString());

        //
        // Setting html elements content.
        //
        entryDragArea.textContent = "☰";

        //
        // Attaching events to html elements.
        //

        // Scrolling on screen touch.
        entryRoot.ontouchstart = _ => {};

        // Dragging for touch screens.
        entryDragArea.ontouchstart = _ => {};

        entryDragArea.onmousedown = event => {
            this.dragStartIndex   = this.findHtmlIndex(entryRoot);
            this.dragCurrentIndex = this.dragStartIndex;

            this.collapseEntry(this.expandedEntry);

            let draggableEntry = entryRoot.cloneNode(true);
            draggableEntry.classList.add("draggable_entry");
            draggableEntry.classList.add("entry_disable_transition");
            this.draggableEntry = draggableEntry;

            this.htmlEntryList.appendChild(draggableEntry);

            let listRect    = this.htmlEntryList.getBoundingClientRect();
            let entryRect   = draggableEntry.getBoundingClientRect();
            let mouseOffset = event.clientY - entryRect.top;
            let top         = (event.clientY - listRect.top - mouseOffset) + "px";
            this.draggableEntry.style.top = top;
            this.draggableTopStart = top;

            entryRoot.classList.add("entry_shadow");

            let onDragTimeout = event => { 
                let dragRect = this.draggableEntry.getBoundingClientRect();
                let dragPos  = dragRect.y + ENTRY_HEIGHT / 2.0;

                let hoverIndex = this.positionToIndex(dragPos);
                let startIndex = this.dragCurrentIndex;
                let endIndex   = this.dragCurrentIndex;

                if (hoverIndex < this.dragCurrentIndex) {
                    for (let i = this.dragCurrentIndex - 1; i >= 0; i--) {
                        const pos = this.indexToPosition(i);
                        if (dragPos > pos + ENTRY_HEIGHT * 0.66666) {
                            break;
                        }

                        endIndex -= 1;

                        const entry = this.htmlEntries[i];
                        this.setEntryPosition(entry, i + 1);
                    }
                } else if (hoverIndex > this.dragCurrentIndex) {
                    for (let i = this.dragCurrentIndex + 1; i < this.htmlEntries.length; i++) {
                        const pos = this.indexToPosition(i);
                        if (dragPos < pos + ENTRY_HEIGHT * 0.33333) {
                            break;
                        }

                        endIndex += 1;

                        const entry = this.htmlEntries[i];
                        this.setEntryPosition(entry, i - 1);
                    }
                }

                if (startIndex === endIndex) {
                    return;
                }

                let entry = this.htmlEntries[startIndex];
                this.htmlEntries.splice(startIndex, 1);
                this.htmlEntries.splice(endIndex, 0, entry);

                { // NOTE(kihau): This will be handled differently.
                    let entry = this.entries[startIndex];
                    this.entries.splice(startIndex, 1);
                    this.entries.splice(endIndex, 0, entry);
                }

                this.setEntryPosition(entry, endIndex);
                this.dragCurrentIndex = endIndex;
            }

            let timeout = null;
            let onDragging = event => {
                let listRect = this.htmlEntryList.getBoundingClientRect();
                let top      = (event.clientY - listRect.top - mouseOffset) + "px";
                this.draggableEntry.style.top = top;

                clearTimeout(timeout);
                timeout = setTimeout(onDragTimeout, 32, event);
            }

            let onDraggingStop = event => {
                clearTimeout(timeout);
                onDragTimeout(event);

                if (this.draggableTopStart === this.draggableEntry.style.top) {
                    this.htmlEntryList.removeChild(this.draggableEntry);
                    entryRoot.classList.remove("entry_shadow");
                } else {
                    this.draggableEntry.classList.remove("entry_disable_transition");
                    this.setEntryPosition(this.draggableEntry, this.dragCurrentIndex);
                    this.draggableEntry.ontransitionend = event => {
                        this.htmlEntryList.removeChild(event.target);
                        entryRoot.classList.remove("entry_shadow");
                    };
                }
                this.draggableEntry = null;


                // TODO(kihau): Proper networking handling.
                if (this.dragStartIndex !== this.dragCurrentIndex) {
                    // let entry = this.entries[this.dragStartIndex];
                    let entry = this.entries[this.dragCurrentIndex];
                    api.playlistMove(entry.id, this.dragStartIndex, this.dragCurrentIndex)
                }

                document.removeEventListener("mousemove", onDragging);
                document.removeEventListener("mouseup",   onDraggingStop);
            };

            document.addEventListener("mousemove", onDragging);
            document.addEventListener("mouseup",   onDraggingStop);
        };

        editButton.onclick = () => {
            console.log("edit button clicked");
            // Tag html elements as content editable on edit button click
            // entryTitle.contentEditable = true;
        };

        deleteButton.onclick = () => {
            let index = this.htmlEntries.findIndex(item => item === entryRoot);
            if (index === -1) {
                console.error("ERROR: Failed to find entry:", entryRoot, "Playlist is out of sync");
                return;
            }

            if (index < 0 || index >= this.entries.length) {
                console.error("ERROR: Delete button click failed for html playlist entry:", entryRoot, "Index:", index, "is out of bounds for array:", this.entries);
                return null;
            }

            // TODO(kihau): Proper networking handling.
            let entry = this.entries[index];
            api.playlistRemove(entry.id, index);
        };

        dropdownButton.onclick = () => {
            if (this.expandedEntry !== entryRoot) {
                this.expandEntry(entryRoot);
            } else {
                this.collapseEntry(entryRoot);
            }
        };

        //
        // Constructing html element structure.
        //
        entryRoot.append(entryTop); {
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
                dropdownButton.append(dropdownSvg);
            }
        }
        entryRoot.append(entryDropdown); {
            entryDropdown.append(dropdownTop); {
                dropdownTop.append(addedByText);
                dropdownTop.append(createdAtText);
            }
            entryDropdown.append(dropdownBottom); {
                dropdownBottom.append(userAvatar); {
                    userAvatar.append(userAvatarImg);
                }
                dropdownBottom.append(userName);
                dropdownBottom.append(creationDate);
            }
        }

        return entryRoot;
    }

    handleServerEvent(action, data) {
        switch (action) {
            case "add": {
                this.addEntry(data);
            } break;

            case "clear": {
                this.clear()
            } break;

            case "remove": {
                this.removeAt(data)
            } break;

            case "shuffle": {
                this.clear();
                this.loadNew(data);
            } break;

            case "move": {
                this.move(data.source_index, data.dest_index);
            } break;

            case "update": {
                // this.update(data.index, data.new_entry);
            } break;

            default: {
                console.error("Unknown playlist action:", action, "with data:", data);
            } break;
        }
    }
}
