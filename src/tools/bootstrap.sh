#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p .cache build .local/assets
if [ ! -d .cache/lvgl/.git ]; then
  git clone --depth 1 --branch v9.3.0 https://github.com/lvgl/lvgl.git .cache/lvgl
fi
test "$(git -C .cache/lvgl rev-parse HEAD)" = c033a98afddd65aaafeebea625382a94020fe4a7
cp /usr/share/fonts/truetype/wqy/wqy-microhei.ttc .local/assets/
cp /usr/share/doc/fonts-wqy-microhei/copyright .local/assets/FONT-LICENSE.txt
if [ ! -f .local/assets/reference.wav ]; then
  curl --fail --location --silent --show-error \
    https://raw.githubusercontent.com/FunAudioLLM/CosyVoice/main/asset/zero_shot_prompt.wav \
    -o .local/assets/reference.wav.tmp
  python3 -c 'from pathlib import Path; b=Path(".local/assets/reference.wav.tmp").read_bytes(); assert len(b)>44 and b[:4]==b"RIFF" and b[8:12]==b"WAVE" and b"data" in b'
  mv .local/assets/reference.wav.tmp .local/assets/reference.wav
fi
printf '%s\n' 'Reference voice: https://github.com/FunAudioLLM/CosyVoice/blob/main/asset/zero_shot_prompt.wav' > .local/assets/VOICE-SOURCE.txt
