import * as api from "./api.js";
import { getById, div, a, span, img, svg, button } from "./util.js";

export { Playlist }

class Playlist {
    constructor() {
        this.htmlEntryList = getById("playlist_entry_list");
        // NOTE: Other items go here like control buttons and input boxes.

        /// Corresponds to the actual playlist entries on the server.
        this.entries = [];

        /// Corresponds to html elements created from the playlist entries.
        this.htmlEntries = [];

        // this.isDraggingEntry = false;
        // this.dragEntryStart  = null;
        // this.dragEntryEnd  = null;
        this.isDragging = null;


        this.controlsRoot   = getById("playlist_controls_root");
        this.dropdownButton = getById("playlist_dropdown_button");
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

        // let applyTransitionUp = (entry, diff) => {
        //     const time = "2000ms";
        //
        //     entry.style.transform  = `translate(0, ${diff}px)`;
        //     entry.style.transition = "transform " + time;
        //
        //     entry.ontransitionend = _ => {
        //         entry.ontransitionend = null;
        //         entry.style.transition = "";
        //         entry.style.transform  = "";
        //
        //         this.htmlEntryList.insertBefore(entryNext, entryRoot);
        //     };
        // };

        let timeout = null; 

        entryDragArea.onmousedown = _ => {
            // entryRoot.style.opacity = 0.3;

            let onDragTimeout = event => {
                let rect = entryRoot.getBoundingClientRect();

                // NOTE(kihau): There must me a nicer way to calculate transition distance.
                const rowGap = 4;
                const distance = rect.height + rowGap;
                const time = 600;

                // NOTE(kihau): Instead of using next/previous sibling element, iterate over the htmlEntries array?

                if (event.y < rect.y) {
                    let count  = 0;

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
                        entry.style.transition = `transform ${time}ms`;
                        entry.ontransitionend = event => {
                            event.target.ontransitionend = null;
                            event.target.style.transition = "";
                            event.target.style.transform  = "";
                        };

                        if (event.y >= rect.y) {
                            break;
                        }
                    }

                    entryRoot.style.transform  = `translate(0, -${distance * count}px)`;
                    entryRoot.style.transition = `transform ${time}ms`;
                    entryRoot.ontransitionend = _ => {
                        entryRoot.ontransitionend = null;
                        entryRoot.style.transition = "";
                        entryRoot.style.transform  = "";

                        this.htmlEntryList.insertBefore(entryRoot, entry);
                    };
                } else if (event.y > rect.y + rect.height) {
                    let count  = 0;

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
                        entry.style.transition = `transform ${time}ms`;
                        entry.ontransitionend = event => {
                            event.target.ontransitionend = null;
                            event.target.style.transition = "";
                            event.target.style.transform  = "";
                        };

                        if (event.y <= rect.y + rect.height) {
                            break;
                        }
                    }

                    entryRoot.style.transform  = `translate(0, ${distance * count}px)`;
                    entryRoot.style.transition = `transform ${time}ms`;
                    entryRoot.ontransitionend = _ => {
                        console.log(entry);
                        entryRoot.ontransitionend = null;
                        entryRoot.style.transition = "";
                        entryRoot.style.transform  = "";

                        this.htmlEntryList.insertBefore(entryRoot, entry.nextSibling);
                    };
                }
            }

            let timeout = null;
            let onDragging = event => {
                clearTimeout(timeout);
                timeout = setTimeout(onDragTimeout, 100, event);
            }

            // let isTransition = false
            // const time = "80ms"
            //
            // let onDragging = event => {
            //     if (isTransition) {
            //         return;
            //     }
            //
            //     let entryNext = entryRoot.nextElementSibling;
            //     let entryPrev = entryRoot.previousElementSibling;
            //
            //     if (entryNext) {
            //         let rect = entryNext.getBoundingClientRect();
            //         if (event.y > rect.y + rect.height / 3.0) {
            //             isTransition = true;
            //
            //             let rootRect = entryRoot.getBoundingClientRect();
            //             let diff = rootRect.y - rect.y;
            //
            //             entryNext.style.transform  = `translate(0, ${diff}px)`;
            //             entryNext.style.transition = "transform " + time;
            //
            //             entryRoot.style.transform  = `translate(0, ${-diff}px)`;
            //             entryRoot.style.transition = "transform " + time;
            //
            //             entryNext.ontransitionend = _ => {
            //                 entryNext.ontransitionend = null;
            //
            //                 entryRoot.style.transition = "";
            //                 entryRoot.style.transform  = "";
            //
            //                 entryNext.style.transition = "";
            //                 entryNext.style.transform  = "";
            //
            //                 this.htmlEntryList.insertBefore(entryNext, entryRoot);
            //                 isTransition = false;
            //             };
            //         }
            //     } 
            //
            //     if (entryPrev) {
            //         let rect = entryPrev.getBoundingClientRect();
            //         if (event.y < rect.y + rect.height / 1.5) {
            //             isTransition = true;
            //
            //             let rootRect = entryRoot.getBoundingClientRect();
            //             let diff = rootRect.y - rect.y;
            //
            //             entryPrev.style.transform  = `translate(0, ${diff}px)`;
            //             entryPrev.style.transition = "transform " + time;
            //
            //             entryRoot.style.transform  = `translate(0, ${-diff}px)`;
            //             entryRoot.style.transition = "transform " + time;
            //
            //             entryPrev.ontransitionend = _ => {
            //                 entryPrev.ontransitionend = null;
            //
            //                 entryRoot.style.transition = "";
            //                 entryRoot.style.transform  = "";
            //
            //                 entryPrev.style.transition = "";
            //                 entryPrev.style.transform  = "";
            //
            //                 this.htmlEntryList.insertBefore(entryPrev, entryNext);
            //                 isTransition = false;
            //             };
            //         }
            //     }
            //
            //     // console.log("Next", next, "Prev", prev, "Event", event);
            //     // console.log("Now", entryRoot.getBoundingClientRect(), "Event", event);
            // };

            let onDraggingStop = _ => {
                this.isDragging = false;
                document.removeEventListener("mousemove", onDragging);
                document.removeEventListener("mouseup",   onDraggingStop);
            };

            this.isDragging = true;


            entryRoot.classList.remove("entry_dropdown_expand");
            document.addEventListener("mousemove", onDragging);
            document.addEventListener("mouseup",   onDraggingStop);
        };

        // Old dragging code something.
        // entryRoot.ondragover = event => { 
        //     if (!this.isDraggingEntry) {
        //         return;
        //     }
        //
        //     this.dragEntryEnd = entryRoot;
        //     // console.log("Drag over event", event, "with", event.target, "but the root is", entryRoot);
        // };
        //
        // entryDragArea.ondragstart = event => {
        //     // console.log("Drag start", event.target)
        //     this.isDraggingEntry = true;
        //     this.dragEntryStart = entryRoot;
        //     this.dragEntryEnd   = entryRoot;
        // };
        //
        // entryDragArea.ondragend = () => { 
        //     if (!this.isDraggingEntry) {
        //         return;
        //     }
        //
        //     this.isDraggingEntry = false;
        //
        //     if (this.dragEntryStart === this.dragEntryEnd) {
        //         return;
        //     }
        //
        //     let startIndex = this.htmlEntries.findIndex(item => item === this.dragEntryStart);
        //     let endIndex   = this.htmlEntries.findIndex(item => item === this.dragEntryEnd);
        //     api.playlistMove(this.entries[startIndex].id, startIndex, endIndex);
        // };


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
