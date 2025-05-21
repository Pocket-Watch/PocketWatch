export function getById(id) {
    return document.getElementById(id);
}

export function show(element) {
    element.classList.add("show");
}

export function hide(element) {
    element.classList.remove("show");
}

export function isHidden(element) {
    element.classList.contains("show")
}

export function div(className) {
    let element = document.createElement("div");
    element.className = className;
    return element;
}

export function a(className, textContent, href = null) {
    let element = document.createElement("a");
    element.className = className;
    element.textContent = textContent;
    if (href) {
        element.href = href;
    } else {
        element.href = textContent;
    }
    return element;
}

export function span(className, textContent) {
    let element = document.createElement("span");
    element.className = className;
    element.textContent = textContent;
    return element;
}

export function img(src) {
    let element = document.createElement("img");
    element.src = src;
    return element;
}

export function isAnimated(url_string) {
    let url = new URL(url_string, window.location.href)
    return url.searchParams.get("ext") === "gif";
}

export function dynamicImg(src) {
    if (!isAnimated(src)) {
        let image = document.createElement("img");
        image.className = "dynamic_image";
        image.src = src;
        return image
    }

    let container  = document.createElement("div");
    let image      = document.createElement("img");
    let canvas     = document.createElement("canvas");

    container.className = "dynamic_image";
    image.src = src;

    image.onload = _ => {
        let w = image.naturalWidth;
        let h = image.naturalHeight;

        canvas.width  = image.width;
        canvas.height = image.height;

        let scaleX = canvas.width / w;
        let scaleY = canvas.height / h;
        let ratio  = Math.max(scaleX, scaleY)

        let offsetX = (canvas.width - w * ratio) / 2.0;
        let offsetY = (canvas.height - h * ratio) / 2.0;

        const ctx = canvas.getContext("2d");
        ctx.drawImage(image, 0, 0, w, h, offsetX, offsetY, w * ratio, h * ratio);
    };

    container.appendChild(image);
    container.appendChild(canvas);

    return container;
}

export function svg(href) {
    let svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    let use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", href);
    svg.appendChild(use);
    return svg;
}

export function button(className, title) {
    let element = document.createElement("button");
    element.className = className;
    element.title = title;
    return element;
}

export function input(className, text, placeholder = null) {
    let element = document.createElement("input");
    element.className = className;
    element.value = text;

    if (placeholder) {
        element.placeholder = placeholder
    }

    return element;
}

export function fileInput(formats) {
    let element = document.createElement("input");
    element.type = "file";

    if (formats) {
        element.accept = formats;
    }

    return element;
}

export function formatTime(seconds) {
    let time = "";
    let hours = 0;
    if (seconds >= 3600) {
        hours = (seconds / 3600) | 0;
        seconds %= 3600;
    }

    let minutes = 0;
    if (seconds >= 60) {
        minutes = (seconds / 60) | 0;
        seconds %= 60;
    }

    if (seconds > 0) {
        seconds |= 0;
    }

    if (hours > 0) {
        let hourSuffix = hours + ":";
        time += (hours < 10) ? "0" + hourSuffix : hourSuffix;
    }

    let minSuffix = minutes + ":";
    time += (minutes < 10) ? "0" + minSuffix : minSuffix;
    time += (seconds < 10) ? "0" + seconds : seconds;
    return time;
}

const PB = 1024 ** 5;
const TB = 1024 ** 4;
const GB = 1024 ** 3;
const MB = 1024 ** 2;
const KB = 1024;

export function formatByteCount(byteCount) {
    if (byteCount > PB) {
        let size = Math.round(byteCount / PB * 10) / 10;
        return size + " PB";
    } else if (byteCount > TB) {
        let size = Math.round(byteCount / TB * 10) / 10;
        return size + " TB";
    } else if (byteCount > GB) {
        let size = Math.round(byteCount / GB * 10) / 10;
        return size + " GB";
    } else if (byteCount > MB) {
        let size = Math.round(byteCount / MB * 10) / 10;
        return size + " MB";
    } else if (byteCount > KB) {
        let size = Math.round(byteCount / KB * 10) / 10;
        return size + " KB";
    } else {
        return byteCount + " B";
    }
}

// This is a wrapper for localStorage (which has only string <-> string mappings)
export class Storage {
    static remove(key) {
        localStorage.removeItem(key);
    }

    static set(key, value) {
        localStorage.setItem(key, value);
    }

    static get(key) {
        return localStorage.getItem(key);
    }

    static getBool(key) {
        let value = localStorage.getItem(key);
        if (value == null) {
            return null;
        }

        return value === "1";
    }

    static setBool(key, value) {
        if (value) {
            localStorage.setItem(key, "1");
        } else {
            localStorage.setItem(key, "0");
        }
    }

    static getNum(key) {
        let value = localStorage.getItem(key);
        if (value == null) {
            return null;
        }

        return Number(value);
    }
}
