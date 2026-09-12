// Sprint-1 stopgap: nothing under server/ existed yet, and P2 (render)
// needed a module to build/test server/internal/render/ against. P3 owns
// this file per FILE_STRUCTURE.md's ownership table and may replace or
// extend it once the coordinator lands; kept intentionally empty of
// dependencies so that merge is a no-op or trivial.
module clarity/server

go 1.22
