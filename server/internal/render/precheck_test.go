package render

import "testing"

const goodSample = `"""
title: Test
description: A simple test scene.
category: general
tags: Circle
"""
from manim import *
import numpy as np


class GeneratedScene(Scene):
    def construct(self):
        c = Circle().move_to(np.array([0, 0, 0]))
        self.play(Create(c))
`

func TestPrecheck(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool
	}{
		{
			name:    "good sample with numpy",
			src:     goodSample,
			wantErr: false,
		},
		{
			name:    "invalid syntax",
			src:     "def broken(:\n    pass\n",
			wantErr: true,
		},
		{
			name: "disallowed import",
			src: "import requests\nfrom manim import *\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        pass\n",
			wantErr: true,
		},
		{
			name: "disallowed import chained after a semicolon",
			src: "from manim import *; import ctypes\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        pass\n",
			wantErr: true,
		},
		{
			name: "os.system call",
			src: "from manim import *\nimport os\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        os.system('ls')\n",
			wantErr: true,
		},
		{
			name: "os.popen call",
			src: "from manim import *\nimport os\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        os.popen('ls')\n",
			wantErr: true,
		},
		{
			name: "shutil.rmtree call",
			src: "from manim import *\nimport shutil\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        shutil.rmtree('/work')\n",
			wantErr: true,
		},
		{
			name: "subprocess import",
			src: "from manim import *\nimport subprocess\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        pass\n",
			wantErr: true,
		},
		{
			name: "eval call",
			src: "from manim import *\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        eval('1+1')\n",
			wantErr: true,
		},
		{
			name: "exec call",
			src: "from manim import *\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        exec('x=1')\n",
			wantErr: true,
		},
		{
			name: "__import__ call",
			src: "from manim import *\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        __import__('os')\n",
			wantErr: true,
		},
		{
			name: "open for writing, text mode",
			src: "from manim import *\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        open('x.txt', 'w')\n",
			wantErr: true,
		},
		{
			name: "open for writing, binary mode",
			src: "from manim import *\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        open('x.bin', 'wb')\n",
			wantErr: true,
		},
		{
			name: "missing GeneratedScene class",
			src: "from manim import *\n" +
				"class SomeOtherScene(Scene):\n    def construct(self):\n        pass\n",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Precheck(tc.src)
			if tc.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr && err.Stage != "precheck" {
				t.Fatalf("expected stage %q, got %q", "precheck", err.Stage)
			}
		})
	}
}
