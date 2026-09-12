package render

import "testing"

const goodSample = `"""
title: Test
description: A simple test scene.
category: general
tags: Circle
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        c = Circle()
        self.play(Create(c))
`

func TestPrecheck(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool
	}{
		{
			name:    "good sample",
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
			name: "os.system call",
			src: "from manim import *\nimport os\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        os.system('ls')\n",
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
			name: "missing GeneratedScene class",
			src: "from manim import *\n" +
				"class SomeOtherScene(Scene):\n    def construct(self):\n        pass\n",
			wantErr: true,
		},
		{
			name: "__import__ call",
			src: "from manim import *\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        __import__('os')\n",
			wantErr: true,
		},
		{
			name: "open for writing",
			src: "from manim import *\n" +
				"class GeneratedScene(Scene):\n    def construct(self):\n        open('x.txt', 'w')\n",
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
