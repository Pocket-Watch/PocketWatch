
let status = document.getElementById("status");
status.style.color = "white";

let processButton = document.getElementById("processButton");
let stopButton = document.getElementById("stopButton");

let outputsList = document.getElementById("outputs");
let inputsList = document.getElementById("inputs");


let mediaStream = null;
let audioContext = null;
let audioSource = null;
let audioWorkletNode = null;

async function processStream() {
    if (!mediaStream) {
        console.warn("Can't execute processStream because mediaStream is", mediaStream)
        return;
    }
    audioContext = new AudioContext();

    // Load the audio worklet processor
    await audioContext.audioWorklet.addModule("js/audio_processor.js");

    // Create an AudioWorkletNode
    audioWorkletNode = new AudioWorkletNode(audioContext, "audio_processor");

    // Create a MediaStreamSource from the microphone stream
    audioSource = audioContext.createMediaStreamSource(mediaStream);

    // Connect the source to the AudioWorkletNode
    audioSource.connect(audioWorkletNode);
    let dest = audioContext.destination;
    console.log("AudioDestination:", dest);
    audioWorkletNode.connect(dest);
}

async function stopStream() {
    if (audioWorkletNode) {
        // Send some "stop" command which was declared in constructor of AudioProcessor
        audioWorkletNode.port.postMessage({ command: 'stop' });
        audioWorkletNode.disconnect();
        audioWorkletNode = null;
    }
}


async function enumerateDevices() {
    const devices = await navigator.mediaDevices.enumerateDevices();
    let [outputs, inputs] = getAudioDevices(devices);
    // Filter and display output devices (including headphones)

    console.log("OUT", outputs)
    console.log("IN", inputs)
    setIntoList(outputsList, outputs)
    setIntoList(inputsList, inputs)
}

function setIntoList(ul, devices) {
    removeAllChildren(ul);
    for (const device of devices) {
        const li = document.createElement("li");
        li.textContent = device.label;
        ul.appendChild(li);
    }
}

function removeAllChildren(htmlElement) {
    for (let i = htmlElement.children.length - 1; i >= 0; i--) {
        htmlElement.removeChild(htmlElement.children[i]);
    }
}

const AUDIO_INPUT = "audioinput";
const AUDIO_OUTPUT = "audiooutput";
function getAudioDevices(devices) {
    console.log(devices)
    let outputDevices = [];
    let inputDevices = [];
    for (const device of devices) {
        switch (device.kind) {
            case AUDIO_OUTPUT:
                outputDevices.push(device);
                break;
            case AUDIO_INPUT:
                inputDevices.push(device);
                break;
        }
    }
    return [outputDevices, inputDevices];
}

async function requestPermission() {
    if (!navigator.mediaDevices) {
        console.warn("mediaDevices are unavailable in insecure contexts (HTTP)")
        return;
    }
    navigator.mediaDevices
        .getUserMedia({ audio: true })
        .then((stream) => {
            mediaStream = stream;
            status.innerText = "Obtained permission";
            console.log(stream);
        })
        .catch((err) => {
            status.innerText = "User refused";
            console.error(`Failed to getUserMedia ${err}`);
        });

}

let selfAudio = new Audio();
document.body.appendChild(selfAudio);

async function stopForwarding() {
    selfAudio.pause();
    selfAudio.setSinkId(-1);
    selfAudio.srcObject = null;
}

async function forwardToSelf() {
    const devices = await navigator.mediaDevices.enumerateDevices();
    let [outputs, inputs] = getAudioDevices(devices);
    if (outputs.length === 0) {
        console.warn("No outputs!");
        return;
    }
    if (inputs.length === 0) {
        console.warn("No inputs!");
        return;
    }
    let inputDeviceId = inputs[0].deviceId;
    let outputDeviceId = outputs[0].deviceId;
    console.log("Attempting to get micStream")
    const micStream = await navigator.mediaDevices.getUserMedia({
        audio: { deviceId: inputDeviceId }
    });
    let audioCtx = new AudioContext();
    let source = audioCtx.createMediaStreamSource(micStream);
    const destination = audioCtx.createMediaStreamDestination();

    source.connect(destination);
    const outputStream = destination.stream;

    // Create an audio element to play the output
    selfAudio.srcObject = outputStream;
    selfAudio.setSinkId(outputDeviceId); // Set the output device
    selfAudio.play();

}

let remoteAudio = document.getElementById("remoteAudio");
let webSocket;

const mediaSource = new MediaSource();
remoteAudio.src = URL.createObjectURL(mediaSource);
let sourceBuffer;

// When the MediaSource is open, create a SourceBuffer
mediaSource.addEventListener('sourceopen', () => {
    // Create a SourceBuffer for the audio format
    sourceBuffer = mediaSource.addSourceBuffer("audio/webm; codecs=opus");
});


const DOMAIN = window.location.host;

const connectButton = document.getElementById("connect");
const disconnectButton = document.getElementById("disconnect");

async function startVoiceChat() {
    const devices = await navigator.mediaDevices.enumerateDevices();
    let [_, inputs] = getAudioDevices(devices);
    if (inputs.length === 0) {
        console.warn("No inputs!");
        return;
    }
    let inputDeviceId = inputs[0].deviceId;
    console.log("Attempting to get micStream")
    const micStream = await navigator.mediaDevices.getUserMedia({
        audio: { deviceId: inputDeviceId }
    });
    console.log("Received micStream", micStream)

    // Initialize WebSocket
    // closeWebSocket(webSocket);
    webSocket = new WebSocket("ws://" + DOMAIN + "/watch/vc");
    connectButton.disabled = true;
    disconnectButton.disabled = false;
    webSocket.onopen = async event => {
        console.log("WebSocket opened /w event:", event);

        const options = { mimeType: "audio/webm; codecs=opus" };
        let mediaRecorder = new MediaRecorder(micStream, options);

        mediaRecorder.ondataavailable = (event) => {
            if (event.data.size > 0) {
                webSocket.send(event.data);
            }
        };

        mediaRecorder.start(500); // Send audio data every given ms
    };

    webSocket.onmessage = async (event) => {
        console.log("WebSocket onmessage called with event.data:", event.data);

        if (!(event.data instanceof Blob)) {
            console.error("Received data is not a Blob:", event.data);
            return;
        }

        const reader = new FileReader();
        reader.onloadend = () => {
            const arrayBuffer = reader.result;

            // Append the audio data to the SourceBuffer
            if (sourceBuffer && !sourceBuffer.updating) {
                sourceBuffer.appendBuffer(arrayBuffer);
                if (remoteAudio.paused) {
                    remoteAudio.play();
                }
            } else {
                console.warn("SourceBuffer is updating, cannot append data yet.");
            }
        };

        // Read the Blob as an ArrayBuffer
        reader.readAsArrayBuffer(event.data);
    };

    webSocket.onerror = async event => {
        console.log("WebSocket ERROR", event);
        discardWebSocket(webSocket);
        connectButton.disabled = false;
        disconnectButton.disabled = true;
    }
}

async function stopVoiceChat() {
    console.log("User disconnected by click");
    remoteAudio.pause();
    discardWebSocket(webSocket);
    connectButton.disabled = false;
    disconnectButton.disabled = true;
    webSocket = null;
}

function close(closeable) {
    if (closeable) {
        closeable.close();
        console.log(closeable, " closed.");
    }
}

function discardWebSocket(socket) {
    if (socket && socket.readyState === WebSocket.OPEN) {
        socket.close(1000, "Closing connection!");
        console.log("WebSocket connection closed.");
    }
}

function main() {
    window.requestPermission = requestPermission;
    window.processStream = processStream;
    window.stopStream = stopStream;
    window.enumerateDevices = enumerateDevices;
    window.forwardToSelf = forwardToSelf;
    window.stopForwarding = stopForwarding;

    window.startVoiceChat = startVoiceChat;
    window.stopVoiceChat = stopVoiceChat;

    outputsList.style.color = "white";
    outputsList.style.fontSize = "40px";
    outputsList.style.backgroundColor = "black";
    inputsList.style.color = "white";
    inputsList.style.fontSize = "40px";
    inputsList.style.backgroundColor = "black";

}
main();
