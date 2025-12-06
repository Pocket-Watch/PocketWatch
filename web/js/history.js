import * as api from "./api.js";
import { getById, div, a, span, img, svg, show, hide, isScrollableVisible } from "./util.js";

export { History }

const MAX_HISTORY_SIZE      = 120;
const DROPDOWN_EXPAND_TIME  = 100;
const BULK_ACTION_DELAY     = 32;
const ENTRY_TRANSITION_TIME = 300;

function createRequestEntry(entry) {
    const requestEntry = {
        url:          entry.url,
        title:        entry.title,
        use_proxy:    entry.use_proxy,
        referer_url:  entry.referer_url,
        subtitles:    entry.subtitles,
        search_video: false,
        is_playlist:  false,
        playlist_skip_count: 0,
        playlist_max_size:   0,
    };

    return requestEntry;
}

class History {
    constructor() {
        this.controlsClearButton    = getById("history_controls_clear");
        this.controlsSettingsButton = getById("history_controls_settings");

        // Corresponds to the actual history entries on the server.
        this.entries = [];

        // Represents the structure of the htmlEntryList post transition while entries are still mid-transition.
        this.htmlEntries = [];

        this.htmlEntryListRoot = getById("history_entry_list_root");

        // HTML DOM with history entries.
        this.htmlEntryList = getById("history_entry_list");

        // Context menu elements.
        this.contextMenu            = getById("history_context_menu");
        this.contextMenuPlayNow     = getById("history_context_play_now");
        this.contextMenuExpand      = getById("history_context_expand");
        this.contextMenuExpandText  = getById("history_context_expand_text");
        this.contextMenuCopyUrl     = getById("history_context_copy_url");
        this.contextMenuCopyEntry   = getById("history_context_copy_entry");
        this.contextMenuAddPlaylist = getById("history_context_add_to_playlist");
        this.contextMenuDelete      = getById("history_context_delete");

        // Selected entry for an open context menu.
        this.contextMenuEntry     = null;
        this.contextMenuHtmlEntry = null;
        this.contextMenuUser      = null;

        // Currently expanded entry. Only one entry is allowed to be expanded at a time.
        this.expandedEntry = null;
    }

    // NOTE(kihau): Attachable history events (similar to the custom player)
    onSettingsClick() {}
    onContextEntryCopy(_entry) {}

    attachHistoryEvents() {
        this.controlsClearButton.onclick    = _ => api.historyClear();
        this.controlsSettingsButton.onclick = _ => this.onSettingsClick();

        this.contextMenu.oncontextmenu = event => {
            event.preventDefault();
            this.hideContextMenu();
        };

        this.htmlEntryList.oncontextmenu = _ => { return false };
        document.addEventListener("click", _ => this.hideContextMenu());

        this.contextMenuPlayNow.onclick     = _ => api.historyPlay(this.contextMenuEntry.id);
        this.contextMenuExpand.onclick      = _ => this.toggleEntryDropdown(this.contextMenuHtmlEntry, this.contextMenuEntry, this.contextMenuUser);
        this.contextMenuCopyUrl.onclick     = _ => navigator.clipboard.writeText(this.contextMenuEntry.url);
        this.contextMenuCopyEntry.onclick   = _ => this.onContextEntryCopy(this.contextMenuEntry);
        this.contextMenuAddPlaylist.onclick = _ => api.historyPlaylistAdd(this.contextMenuEntry.id);
        this.contextMenuDelete.onclick      = _ => api.historyDelete(this.contextMenuEntry.id);
    }

    hideContextMenu() {
        if (this.contextMenuHtmlEntry) {
            this.contextMenuHtmlEntry.classList.remove("highlight");
        }

        this.contextMenuEntry     = null;
        this.contextMenuHtmlEntry = null;
        this.contextMenuUser      = null;

        hide(this.contextMenu);
    }

    showContextMenu(event, entry, htmlEntry, user) {
        if (this.contextMenuHtmlEntry) {
            this.contextMenuHtmlEntry.classList.remove("highlight");
        }

        show(this.contextMenu);

        if (this.expandedEntry === htmlEntry) {
            this.contextMenuExpandText.textContent = "Collapse";
            this.contextMenuExpand.classList.add("expanded");
        } else {
            this.contextMenuExpandText.textContent = "Expand";
            this.contextMenuExpand.classList.remove("expanded");
        }

        const entryRect = htmlEntry.getBoundingClientRect();
        const rootRect  = this.htmlEntryListRoot.getBoundingClientRect();
        const listRect  = this.htmlEntryList.getBoundingClientRect();
        const height    = this.contextMenu.offsetHeight;
        const width     = this.contextMenu.offsetWidth;

        let contextMenuX = event.clientX;
        let protrusion = contextMenuX + width - entryRect.right;
        if (protrusion > 0) {
            contextMenuX -= protrusion;
        }

        let contextMenuY = event.clientY;
        protrusion = contextMenuY + height - rootRect.bottom;
        if (protrusion > 0) {
            contextMenuY -= protrusion;
        }

        this.contextMenu.style.left = (contextMenuX - entryRect.left) + "px";
        this.contextMenu.style.top  = (contextMenuY - listRect.top)   + "px";

        this.contextMenuEntry     = entry;
        this.contextMenuHtmlEntry = htmlEntry;
        this.contextMenuUser      = user;

        this.contextMenuHtmlEntry.classList.add("highlight");
    }

    expandEntry(htmlEntry, entry, user) {
        if (this.expandedEntry) {
            this.expandedEntry.classList.remove("expand");

            let expanded = this.expandedEntry;
            let dropdown = expanded.getElementsByClassName("entry_dropdown")[0];
            setTimeout(_ => expanded.removeChild(dropdown), DROPDOWN_EXPAND_TIME);
        }

        if (htmlEntry) {
            let dropdown = this.createEntryDropdown(entry, user);

            this.expandedEntry = htmlEntry;
            this.expandedEntry.appendChild(dropdown);

            window.getComputedStyle(dropdown).height;

            this.expandedEntry.classList.add("expand");
        }
    }

    collapseEntry(htmlEntry) {
        if (htmlEntry !== this.expandedEntry) {
            return;
        }

        if (this.expandedEntry) {
            this.expandedEntry.classList.remove("expand");

            let expanded = this.expandedEntry;
            let dropdown = expanded.getElementsByClassName("entry_dropdown")[0];
            setTimeout(_ => expanded.removeChild(dropdown), DROPDOWN_EXPAND_TIME);
        }

        this.expandedEntry = null;
    }

    toggleEntryDropdown(htmlEntry, entry, user) {
        if (this.expandedEntry !== htmlEntry) {
            this.expandEntry(htmlEntry, entry, user);
        } else {
            this.collapseEntry(htmlEntry);
        }
    }

    createEntryDropdown(entry, user) {
        let entryDropdown  = div("entry_dropdown");
        let proxyRoot      = div("entry_dropdown_proxy_root");
        let proxyToggle    = widget_toggle(null, "Enable proxy", entry.use_proxy, true);
        let proxyReferer   = widget_input(null, "Referer", entry.referer_url, true);

        let infoLabelsTop  = div("entry_dropdown_info_labels");
        let createdByLabel = span("entry_dropdown_created_by_label", "Created by"); 
        let createdAtLabel = span("entry_dropdown_created_at_label", "Created at");

        let infoLabelsBot  = div("entry_dropdown_info_labels");
        let subsCountLabel = span("entry_dropdown_subtitle_count_label", "Attached subtitles");
        let lastSetAtLabel = span("entry_dropdown_last_set_at_label", "Last set at");

        let createdAt      = new Date(entry.created_at);
        let lastSetAt      = new Date(entry.last_set_at);
        let userAvatarImg  = img(user.avatar);

        let infoContentTop = div("entry_dropdown_info_content");
        let userAvatar     = div("entry_dropdown_user_avatar");
        let userName       = span("entry_dropdown_user_name", user.username);
        let createdAtDate  = span("entry_dropdown_created_at_date", createdAt.toLocaleString());

        let infoContentBot = div("entry_dropdown_info_content");
        let subsCount      = span("entry_dropdown_subtitle_count", "0 subtitles");
        let lastSetAtDate  = span("entry_dropdown_last_set_at_date", lastSetAt.toLocaleString());


        if (!entry.subtitles || entry.subtitles.length === 0) {
            subsCount.textContent = "No subtitles";
        } else if (entry.subtitles.length === 1) {
            subsCount.textContent = entry.subtitles.length + " subtitle";
        } else {
            subsCount.textContent = entry.subtitles.length + " subtitles";
        }


        entryDropdown.append(proxyRoot); {
            proxyRoot.append(proxyToggle);
            proxyRoot.append(proxyReferer);
        }

        entryDropdown.append(infoLabelsTop); { 
            infoLabelsTop.append(createdByLabel);
            infoLabelsTop.append(createdAtLabel);
        }
        entryDropdown.append(infoContentTop); {
            infoContentTop.append(userAvatar); {
                userAvatar.append(userAvatarImg);
            }
            infoContentTop.append(userName);
            infoContentTop.append(createdAtDate);
        }

        entryDropdown.append(infoLabelsBot); { 
            infoLabelsBot.append(subsCountLabel);
            infoLabelsBot.append(lastSetAtLabel);
        }
        entryDropdown.append(infoContentBot); {
            infoContentBot.append(subsCount);
            infoContentBot.append(lastSetAtDate);
        }

        return entryDropdown;
    }

    createHtmlEntry(entry, user) {
        let entryRoot      = div("history_entry");
        let entryTop       = div("history_entry_top"); 
        let entryThumbnail = div("history_entry_thumbnail");
        let thumbnailSrc   = entry.thumbnail ? entry.thumbnail : "img/thumbnail_placeholder.png";
        let thumbnailPlay  = svg("svg/main_icons.svg#thumbnail_play");
        let thumbnailImg   = img(thumbnailSrc, true);
        let entryInfo      = div("history_entry_info");
        let entryTitle     = span("history_entry_title", entry.title);
        let entryUrl       = a("history_entry_url", entry.url);
        let dropdownSvg    = svg("svg/main_icons.svg#dropdown");
        let dropdownButton = div("entry_dropdown_button");

        //
        // Attaching events to html elements.
        //

        entryThumbnail.onclick = _ => api.historyPlay(entry.id);
        dropdownButton.onclick = _ => this.toggleEntryDropdown(entryRoot, entry, user);

        entryRoot.oncontextmenu = event => {
            event.preventDefault();

            if (this.contextMenuEntry && this.contextMenuEntry.id === entry.id) {
                this.hideContextMenu();
            } else {
                this.showContextMenu(event, entry, entryRoot, user);
            }
        };

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

    findUser(users, id) {
        const user = users.find(user => user.id === id);
        if (user) {
            return user;
        } else {
            const dummy = {
                id: 0,
                username: "<Unknown user>",
                avatar: "img/default_avatar.png",
                online: false,
            };

            return dummy;
        }
    }

    load(entries, users) {
        if (!entries) {
            console.warn("WARN: Entry list passed to History::load is null.");
            return;
        }

        let length = Math.min(entries.length, MAX_HISTORY_SIZE);
        for (let i = 0; i < entries.length; i++) {
            const entry = entries[i];

            const user      = this.findUser(users, entry.user_id);
            const htmlEntry = this.createHtmlEntry(entry, user);

            this.entries.push(entry);
            this.htmlEntries.push(htmlEntry);
            this.htmlEntryList.appendChild(htmlEntry);

            setTimeout(_ => {
                window.getComputedStyle(htmlEntry).marginLeft;
                show(htmlEntry);
            }, i * BULK_ACTION_DELAY);
        }
    }

    add(entry, users) {
        const user      = this.findUser(users, entry.user_id);
        const htmlEntry = this.createHtmlEntry(entry, user);

        this.entries.push(entry);
        this.htmlEntries.push(htmlEntry);

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

    delete(entryId) {
        let index = this.entries.findIndex(item => item.id === entryId);
        if (index === -1) {
            console.error("ERROR: History::delete failed. Entry with id", entryId, "is not in the history.");
            return;
        }

        if (this.contextMenuEntry && entryId === this.contextMenuEntry.id) {
            this.hideContextMenu();
        }

        let htmlEntry = this.htmlEntries[index];

        this.entries.splice(index, 1);
        this.htmlEntries.splice(index, 1);

        hide(htmlEntry)
        setTimeout(_ => this.htmlEntryList.removeChild(htmlEntry), ENTRY_TRANSITION_TIME);
    }

    clear() {
        for (let i = 0; i < this.htmlEntries.length; i++) {
            const htmlEntry = this.htmlEntries[i];
            setTimeout(_ => {
                hide(htmlEntry);
                setTimeout(_ => this.htmlEntryList.removeChild(htmlEntry), ENTRY_TRANSITION_TIME);
            }, i * BULK_ACTION_DELAY);
        }

        this.htmlEntries   = [];
        this.entries       = [];
    }
}
