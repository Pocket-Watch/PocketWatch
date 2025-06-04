import * as api from "./api.js";
import { getById, div, a, span, img, svg, show } from "./util.js";

export { History }

const MAX_HISTORY_SIZE = 80

class History {
    constructor() {
        this.controlsClearButton    = getById("history_controls_clear");
        this.controlsSettingsButton = getById("history_controls_settings");

        // Corresponds to the actual history entries on the server.
        this.entries = [];

        // Represents the structure of the htmlEntryList post transition while entries are still mid transition.
        this.htmlEntries = [];

        // HTML DOM with history entries.
        this.htmlEntryList = getById("history_entry_list");
    }

    // NOTE(kihau): Attachable history events (similar to the custom player)
    onSettingsClick() {}

    attachHistoryEvents() {
        this.controlsClearButton.onclick    = _ => api.historyClear();
        this.controlsSettingsButton.onclick = _ => this.onSettingsClick();
    }

    createHtmlEntry(entry) {
        let entryRoot      = div("history_entry");
        let entryTop       = div("history_entry_top"); 
        let entryThumbnail = div("history_entry_thumbnail");
        let thumbnailSrc   = entry.thumbnail ? entry.thumbnail : "img/thumbnail_placeholder.png";
        let thumbnailPlay  = svg("svg/main_icons.svg#thumbnail_play")
        let thumbnailImg   = img(thumbnailSrc);
        let entryInfo      = div("history_entry_info");
        let entryTitle     = span("history_entry_title", entry.title);
        let entryUrl       = a("history_entry_url", entry.url);
        let dropdownSvg    = svg("svg/main_icons.svg#dropdown");
        let dropdownButton = div("history_dropdown_button");

        //
        // Attaching events to html elements.
        //

        entryThumbnail.onclick = _ => console.warn("TODO");
        dropdownButton.onclick = _ => console.warn("TODO")

        //
        // Constructing html element structure.
        //
        entryRoot.append(entryTop); {
            entryTop.append(entryThumbnail); {
                entryThumbnail.append(thumbnailImg);
                entryThumbnail.append(thumbnailPlay);
            }
            entryTop.append(entryInfo); {
                entryInfo.append(entryTitle);
                entryInfo.append(entryUrl);
            }
            entryTop.append(dropdownButton); {
                dropdownButton.append(dropdownSvg);
            }
        }

        return entryRoot;
    }

    add(entry) {
        this.entries.push(entry);

        const htmlEntry = this.createHtmlEntry(entry);
        this.htmlEntries.push(htmlEntry);
        // this.htmlEntryList.appendChild(htmlEntry);

        let first = this.htmlEntryList.firstChild;
        this.htmlEntryList.insertBefore(htmlEntry, first);

        window.getComputedStyle(htmlEntry).marginLeft;
        show(htmlEntry);

        if (this.entries.length > MAX_HISTORY_SIZE) {
            this.entries.shift();
            let removed = this.htmlEntries.shift();
            this.htmlEntryList.removeChild(removed);
        }
    }

    clear() {
        this.entries     = [];
        this.htmlEntries = [];

        let list = this.htmlEntryList;
        while (list.lastChild) {
            list.removeChild(list.lastChild);
        }
    }
}
