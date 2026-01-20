import yt_dlp, json, pprint


ydl_opts = {
    #'cookiesfrombrowser': ['brave','Default']  # Options: 'chrome', 'firefox', 'edge', 'brave'
}

url = "https://www.tiktok.com/@megaamerican/video/7596156950866332942?is_from_webapp=1&sender_device=pc"

try:
    with yt_dlp.YoutubeDL(ydl_opts) as ydl:
        #ydl.download([url])
        info_dict = ydl.extract_info(url, download=False)
        #pprint.pprint(json.dumps(info_dict, indent=2, default=str))
        print(info_dict.keys())
        print(info_dict["title"])

except Exception as e:
    print(f"An error occurred: {e}")