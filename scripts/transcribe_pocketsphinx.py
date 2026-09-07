#!/usr/bin/env python3
"""Offline, local speech-to-text reference implementation for SentryMed Opti.

Contract expected by the Go server (internal/server/recordings.go): invoked as
    <command from System -> Clinic settings> <path-to-audio-file>
must print the transcript to stdout and exit 0 on success, or print a short
error to stderr and exit non-zero on failure. No network access is used or
required anywhere in this script -- the audio never leaves the machine.

Setup (one-time, on the clinic computer that runs the SentryMed server):
    1. Install ffmpeg (used only to normalize the browser's recorded audio
       to 16kHz mono PCM, a format pocketsphinx understands):
         Fedora:  sudo dnf install ffmpeg
         Windows/macOS: https://ffmpeg.org/download.html
    2. pip install pocketsphinx
       (pocketsphinx bundles a small English acoustic + language model
       directly in the pip package -- no separate model download needed.)
    3. In SentryMed, go to System -> Clinic -> Consultation recording and
       set the transcription command to:
         python3 /path/to/sentrymedoptidesktop/scripts/transcribe_pocketsphinx.py
       then enable transcription.

Limitations (read before relying on this for clinical documentation):
    - English only. pocketsphinx's bundled model does not cover French,
      Haitian Creole, Spanish or Portuguese. A clinic that needs another
      language must swap in a different local engine behind the same
      contract (this script is a reference, not the only option).
    - Accuracy is modest by modern standards (pocketsphinx is decades-old
      HMM/GMM technology, not a neural model). Treat every transcript as a
      draft that a clinician must review and correct before it becomes part
      of the official record -- the SentryMed UI enforces exactly that by
      always showing transcripts as editable, never as a final note.
"""
import subprocess
import sys
import tempfile
import os


def fail(message: str) -> None:
    print(message, file=sys.stderr)
    sys.exit(1)


def main() -> None:
    if len(sys.argv) != 2:
        fail("usage: transcribe_pocketsphinx.py <audio-file>")
    audio_path = sys.argv[1]
    if not os.path.isfile(audio_path):
        fail(f"audio file not found: {audio_path}")

    try:
        from pocketsphinx import Decoder, get_model_path
    except ImportError:
        fail("pocketsphinx is not installed. Run: pip install pocketsphinx")

    wav_fd, wav_path = tempfile.mkstemp(suffix=".wav")
    os.close(wav_fd)
    try:
        result = subprocess.run(
            ["ffmpeg", "-y", "-i", audio_path, "-ar", "16000", "-ac", "1", "-f", "wav", wav_path],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
        )
        if result.returncode != 0:
            fail(f"ffmpeg could not convert the recording: {result.stderr.decode(errors='replace')[-500:]}")

        model_path = get_model_path()
        base = os.path.join(model_path, "en-us")
        decoder = Decoder(
            hmm=os.path.join(base, "en-us"),
            lm=os.path.join(base, "en-us.lm.bin"),
            dict=os.path.join(base, "cmudict-en-us.dict"),
        )
        with open(wav_path, "rb") as handle:
            handle.read(44)  # skip the 44-byte canonical WAV header
            buf = handle.read()
        decoder.start_utt()
        decoder.process_raw(buf, no_search=False, full_utt=True)
        decoder.end_utt()
        hypothesis = decoder.hyp()
        print(hypothesis.hypstr if hypothesis else "")
    finally:
        try:
            os.remove(wav_path)
        except OSError:
            pass


if __name__ == "__main__":
    main()
