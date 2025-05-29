import yt_dlp
import time
import json
import http.server

class YoutubeVideo:
    def __init__(self, id, title, thumbnail, original_url, url):
        self.id           = id
        self.title        = title
        self.thumbnail    = thumbnail
        self.original_url = original_url
        self.url          = url

    def json(self):
        return json.dumps(self, default=lambda obj: obj.__dict__)

ydl_opts = {
    'format': '234',
    'extractor_args': {
        'youtube': {
            'player_client': ['ios']
        }
    },
    'playlist_items': '1',
    'noplaylist': True,
}

ydl = yt_dlp.YoutubeDL(ydl_opts)

def get_youtube_data(query: str):
    start = time.time()
    info = ydl.extract_info(query, download=False)
    end = time.time()
    print("Elapsed extract: " + str(end - start))

    if info is None:
        return

    entries = info.get("entries")
    if entries is not None:
        entry = entries[0]

        id           = entry.get("id")
        title        = entry.get("title")
        thumbnail    = entry.get("thumbnail")
        original_url = entry.get("original_url")
        url          = entry.get("url")
    else:
        id           = info.get("id")
        title        = info.get("title")
        thumbnail    = info.get("thumbnail")
        original_url = info.get("original_url")
        url          = info.get("url")

    return YoutubeVideo(id, title, thumbnail, original_url, url)

class InternalServer(http.server.SimpleHTTPRequestHandler):
    def do_POST(self):
        if self.path == '/youtube/fetch':
            request = json.loads(self.rfile.read1())
            data = get_youtube_data(request["query"])

            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            response = bytes(data.json(), "utf-8")
            self.wfile.write(response)
        else:
            self.send_response(404)
            self.end_headers()

hostName = "localhost"
serverPort = 2345

webServer = http.server.HTTPServer((hostName, serverPort), InternalServer)
print("Running an internal helper server at http://%s:%s" % (hostName, serverPort))

webServer.serve_forever()
