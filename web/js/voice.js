
let status = document.getElementById("status");
status.style.color = "white";

let processButton = document.getElementById("processButton");
let stopButton = document.getElementById("stopButton");


let mediaStream = null;
let audioContext = null;
let audioSource = null;
let audioWorkletNode = null;

async function processStream() {
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

function requestPermission() {
    navigator.mediaDevices
        .getUserMedia({ audio: true })
        .then((stream) => {
            mediaStream = stream;
            status.innerText = "Obtained MediaStream";
            console.log(mediaStream);
        })
        .catch((err) => {
            status.innerText = "User refused";
            console.error(`Failed to getUserMedia ${err}`);
        });
}

function main() {
    window.requestPermission = requestPermission;
    window.processStream = processStream;
    window.stopStream = stopStream;

}
main();
