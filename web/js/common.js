import { getCssNumber } from "./util.js";
import * as api from "./api.js";

const DROPDOWN_EXPAND_TIME = getCssNumber("--dropdown_expand_time", "ms")
const DOMAIN_URL = window.location.protocol + "//" + window.location.host;

export function toggleEntryDropdown(self, htmlEntry, entry, user) {
    if (self.expandedEntry !== htmlEntry) {
        expandEntry(self, htmlEntry, entry, user);
    } else {
        collapseEntry(self, htmlEntry);
    }
}

function scheduleDropdownRemove(htmlEntry) {
    let dropdowns = htmlEntry.getElementsByClassName("entry_dropdown");
    setTimeout(_ => {
        if (htmlEntry.classList.contains("expand")) {
            return;
        }

        htmlEntry.classList.remove("collapse");
        for (let i = 0; i < dropdowns.length; i++) {
            htmlEntry.removeChild(dropdowns[i]);
        }
    }, DROPDOWN_EXPAND_TIME);
}


export function expandEntry(self, htmlEntry, entry, user) {
    if (self.expandedEntry) {
        self.expandedEntry.classList.remove("expand");
        self.expandedEntry.classList.add("collapse");
        scheduleDropdownRemove(self.expandedEntry)
    }

    if (htmlEntry) {
        let dropdowns = htmlEntry.getElementsByClassName("entry_dropdown");

        htmlEntry.classList.add("expand");
        htmlEntry.classList.remove("collapse");

        if (dropdowns.length === 0) {
            let dropdown = createEntryDropdown(entry, user);
            htmlEntry.appendChild(dropdown);
            window.getComputedStyle(dropdown).height;
        }

        self.expandedEntry = htmlEntry;
    }
}

export function collapseEntry(self, htmlEntry) {
    if (htmlEntry !== self.expandedEntry) {
        return;
    }

    if (self.expandedEntry) {
        self.expandedEntry.classList.remove("expand");
        self.expandedEntry.classList.add("collapse");
        scheduleDropdownRemove(self.expandedEntry)
    }

    self.expandedEntry = null;
}

export function createEntryDropdown(entry, user) {
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

export async function shareResourceUrl(url) {
    if (!url) return;
    let response = await api.shareResource(url, 600);
    if (response.checkError()) return;
    let sharedUrl = DOMAIN_URL + response.json.shared_path;
    await navigator.clipboard.writeText(sharedUrl);
}
