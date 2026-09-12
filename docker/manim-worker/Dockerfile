FROM python:3.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    texlive-latex-base texlive-latex-extra texlive-fonts-recommended texlive-science \
    dvisvgm ffmpeg libcairo2-dev libpango1.0-dev pkg-config build-essential \
 && rm -rf /var/lib/apt/lists/*
RUN pip install --no-cache-dir manim
WORKDIR /work
