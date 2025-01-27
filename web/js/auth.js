let encoder = new TextEncoder();
export async function sha256(text) {
    let buffer = encoder.encode(text);
    const hashBuffer = await crypto.subtle.digest("SHA-256", buffer);
    const hashArray = new Uint8Array(hashBuffer);
    let hexHash = "";
    for (let i = 0; i < hashArray.length; i++) {
        hexHash += hashArray[i].toString(16).padStart(2, '0');
    }
    return hexHash;
}