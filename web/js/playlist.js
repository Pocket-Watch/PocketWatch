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
    }

    addEntry(entry) {
        this.entries.push(entry);

        const htmlEntry = this.createHtmlEntry(entry);
        this.htmlEntries.push(htmlEntry);
        this.htmlEntryList.appendChild(htmlEntry);
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
        dropdownButton.textContent = "▼";

        //
        // Attaching events to html elements.
        //
        entryDragArea.onclick = () => {
            console.log("dragged area was clicked");
        };

        editButton.onclick = () => {
            console.log("edit button clicked");
            // Tag html elements as content editable on edit button click
            // entryTitle.contentEditable = true;
        };

        deleteButton.onclick = () => {
            console.log("delete button clicked");
        };

        dropdownButton.onclick = () => {
            entryDiv.classList.toggle("entry_dropdown_expand");
        };

        //
        // Constructing html element structure.
        //
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
            entryTop.append(dropdownButton);
        }
        entryDiv.append(entryDropdown); {
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

        return entryDiv;
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
                // this.removeAt(data)
            } break;

            case "shuffle": {
                // this.loadNew(data);
            } break;

            case "move": {
                // this.move(data.source_index, data.dest_index);
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
