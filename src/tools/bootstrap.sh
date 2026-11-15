#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p .cache build .local/assets/app-icons
cp assets/app-icons/*.png .local/assets/app-icons/
if [ ! -d .cache/lvgl/.git ]; then
  git clone --depth 1 --branch v9.3.0 https://github.com/lvgl/lvgl.git .cache/lvgl
fi
sh tools/prepare-lvgl.sh
cp /usr/share/fonts/truetype/wqy/wqy-microhei.ttc .local/assets/
cp /usr/share/doc/fonts-wqy-microhei/copyright .local/assets/FONT-LICENSE.txt
emoji_source=https://raw.githubusercontent.com/google/fonts/cc39c4c2c304bc368c9b7c311ea2bcc6013299fe/ofl/notoemoji
if [ ! -f .local/assets/NotoEmoji.ttf ]; then
  curl --fail --location --silent --show-error \
    "$emoji_source/NotoEmoji%5Bwght%5D.ttf" -o .local/assets/NotoEmoji.ttf.tmp
  mv .local/assets/NotoEmoji.ttf.tmp .local/assets/NotoEmoji.ttf
fi
printf '%s\n' 'de6c18832938afc99caf132b39d6a30a19bac7f2e812e28db2535b4608d27551  .local/assets/NotoEmoji.ttf' | sha256sum -c - >/dev/null
if [ ! -f .local/assets/EMOJI-FONT-LICENSE.txt ]; then
  curl --fail --location --silent --show-error \
    "$emoji_source/OFL.txt" -o .local/assets/EMOJI-FONT-LICENSE.txt.tmp
  mv .local/assets/EMOJI-FONT-LICENSE.txt.tmp .local/assets/EMOJI-FONT-LICENSE.txt
fi
printf '%s\n' "Noto Emoji (SIL OFL 1.1): $emoji_source" > .local/assets/EMOJI-FONT-SOURCE.txt
if [ ! -f .local/assets/reference.wav ]; then
  curl --fail --location --silent --show-error \
    https://raw.githubusercontent.com/FunAudioLLM/CosyVoice/main/asset/zero_shot_prompt.wav \
    -o .local/assets/reference.wav.tmp
  python3 -c 'from pathlib import Path; b=Path(".local/assets/reference.wav.tmp").read_bytes(); assert len(b)>44 and b[:4]==b"RIFF" and b[8:12]==b"WAVE" and b"data" in b'
  mv .local/assets/reference.wav.tmp .local/assets/reference.wav
fi
printf '%s\n' 'Reference voice: https://github.com/FunAudioLLM/CosyVoice/blob/main/asset/zero_shot_prompt.wav' > .local/assets/VOICE-SOURCE.txt
