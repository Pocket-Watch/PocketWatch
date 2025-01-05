import * as api from "./api.js";
import { getById, div, a, span, img, svg, button } from "./util.js";

export { Playlist }

class Playlist {
    constructor() {
        this.htmlEntryListWrap = getById("playlist_entry_list_wrap");
        this.htmlEntryList     = getById("playlist_entry_list");
        this.controlsRoot      = getById("playlist_controls_root");
        this.dropdownButton    = getById("playlist_dropdown_button");
        this.draggableVisual   = getById("playlist_draggable_visual_entry");

        // NOTE: Other items go here like control buttons and input boxes.

        /// Corresponds to the actual playlist entries on the server.
        this.entries = [];

        /// Corresponds to html elements created from the playlist entries.
        this.htmlEntries = [];
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
        this.htmlEntries.push(htmlEntry);
        this.htmlEntryList.appendChild(htmlEntry);
    }

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
    }

    // insertAt(entry, index) {
    //     if (typeof index !== "number") {
    //         console.error("ERROR: Playlist::insertAt failed. The input index:", index, "is invalid.");
    //         return;
    //     }
    //
    //     if (index < 0 || index >= this.entries.length) {
    //         console.error("ERROR: Playlist::insertAt failed. Index:", index, "is out of bounds for array:", this.entries);
    //         return;
    //     }
    // }

    move(sourceIndex, destIndex) {
        let sourceHtmlEntry = this.htmlEntries[sourceIndex];
        let sourceEntry     = this.entries[sourceIndex];

        let destHtmlEntry = this.htmlEntries[destIndex];

        this.removeAt(sourceIndex);

        this.entries.splice(destIndex, 0, sourceEntry);
        this.htmlEntries.splice(destIndex, 0, sourceHtmlEntry);
        this.htmlEntryList.insertBefore(sourceHtmlEntry, destHtmlEntry)
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
        // entryDragArea.draggable = true;

        //
        // Attaching events to html elements.
        //

        // Scrolling on screen touch.
        entryRoot.ontouchstart = _ => {};

        // Dragging for touch screens.
        entryDragArea.ontouchstart = _ => {};

        entryDragArea.onmousedown = event => {
            entryRoot.classList.remove("entry_dropdown_expand");

            let visual = entryRoot.cloneNode(true);
            this.draggableVisual.appendChild(visual);

            let wrapRect    = this.htmlEntryListWrap.getBoundingClientRect();
            let entryRect   = entryRoot.getBoundingClientRect();
            let mouseOffset = event.clientY - entryRect.top + 2.0;
            let top         = event.clientY - wrapRect.top - mouseOffset;
            this.draggableVisual.style.top = top + "px";

            // let wrapRect   = this.htmlEntryListWrap.getBoundingClientRect();
            // let visualRect = this.draggableVisual.getBoundingClientRect();
            // let height     = visualRect.height;
            // let top        = event.clientY - wrapRect.top - height / 2.0;
            // this.draggableVisual.style.top = top + "px";

            entryRoot.style.opacity = 0.3;

            let onDragTimeout = event => {
                let rect = entryRoot.getBoundingClientRect();

                // NOTE(kihau): There must me a nicer way to calculate transition distance.
                const rowGap = 4;
                const distance = rect.height + rowGap;
                const transitionTime = 120;

                // TODO?(kihau): Instead of using next/previous sibling element, iterate over the htmlEntries array?

                // TODO(kihau): Add do not swap when mouse enter the entry but rather when its y > entry y + height / 2.0?

                if (event.y < rect.y) {
                    let count = 0;

                    let entry = entryRoot;
                    while (true) {
                        if (!entry.previousElementSibling) {
                            break;
                        }

                        entry = entry.previousElementSibling 
                        rect  = entry.getBoundingClientRect();

                        if (event.y > rect.y + rect.height) {
                            break;
                        }

                        count += 1;

                        entry.style.transform  = `translate(0, ${distance}px)`;
                        entry.style.transition = `transform ${transitionTime}ms`;
                        entry.ontransitionend = event => {
                            event.target.ontransitionend = null;
                            event.target.style.transition = "";
                            event.target.style.transform  = "";
                        };

                        if (event.y >= rect.y) {
                            break;
                        }
                    }

                    if (count == 0) {
                        return;
                    }

                    entryRoot.style.transform  = `translate(0, -${distance * count}px)`;
                    entryRoot.style.transition = `transform ${transitionTime}ms`;
                    entryRoot.ontransitionend = _ => {
                        entryRoot.ontransitionend = null;
                        entryRoot.style.transition = "";
                        entryRoot.style.transform  = "";

                        this.htmlEntryList.insertBefore(entryRoot, entry);
                    };
                } else if (event.y > rect.y + rect.height) {
                    let count = 0;

                    let entry = entryRoot;
                    while (true) {
                        if (!entry.nextElementSibling) {
                            break;
                        }

                        entry = entry.nextElementSibling 
                        rect  = entry.getBoundingClientRect();

                        if (event.y < rect.y) {
                            break;
                        }

                        count += 1;

                        entry.style.transform  = `translate(0, -${distance}px)`;
                        entry.style.transition = `transform ${transitionTime}ms`;
                        entry.ontransitionend = event => {
                            event.target.ontransitionend = null;
                            event.target.style.transition = "";
                            event.target.style.transform  = "";
                        };

                        if (event.y <= rect.y + rect.height) {
                            break;
                        }
                    }

                    if (count == 0) {
                        return;
                    }

                    entryRoot.style.transform  = `translate(0, ${distance * count}px)`;
                    entryRoot.style.transition = `transform ${transitionTime}ms`;
                    entryRoot.ontransitionend = _ => {
                        entryRoot.ontransitionend = null;
                        entryRoot.style.transition = "";
                        entryRoot.style.transform  = "";

                        this.htmlEntryList.insertBefore(entryRoot, entry.nextSibling);
                    };
                }
            }

            let timeout = null;
            let onDragging = event => {
                let wrapRect = this.htmlEntryListWrap.getBoundingClientRect();
                let top      = event.clientY - wrapRect.top - mouseOffset;
                this.draggableVisual.style.top = top + "px";

                clearTimeout(timeout);
                timeout = setTimeout(onDragTimeout, 16, event);
            }

            let onDraggingStop = _ => {
                entryRoot.style.opacity = null;
                this.draggableVisual.removeChild(this.draggableVisual.firstChild);

                // TOOD(kihau): Smooth transition to the correct spot instead of rapid entry jump.
                // let entryRect = entryRoot.getBoundingClientRect();
                // let visualRect = this.draggableVisual.getBoundingClientRect();
                // let distance = entryRect.y - visualRect.y;
                //
                // this.draggableVisual.style.transform  = `translate(0, ${distance}px)`;
                // this.draggableVisual.style.transition = `transform 120ms`;
                // this.draggableVisual.ontransitionend = event => {
                //     this.draggableVisual.removeChild(this.draggableVisual.firstChild);
                //
                //     this.draggableVisual.style.transform  = "";
                //     this.draggableVisual.style.transition = "";
                //
                //     entryRoot.style.opacity = null;
                // };

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

            let entry = this.entries[index];
            api.playlistRemove(entry.id, index);
        };

        dropdownButton.onclick = () => {
            entryRoot.classList.toggle("entry_dropdown_expand");
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
                // this.clear()
            } break;

            case "remove": {
                this.removeAt(data)
            } break;

            case "shuffle": {
                // this.loadNew(data);
            } break;

            case "move": {
                this.move(data.source_index, data.dest_index);
            } break;

            case "update": {
                // this.move(data.source_index, data.dest_index);
            } break;

            default: {
                console.error("Unknown playlist action:", action, "with data:", data);
            } break;
        }
    }
}
