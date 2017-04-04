class Newt < Formula
  desc "Package, build and installation management system for Mynewt OS embedded applications"
  homepage "https://github.com/cwanda/incubator-mynewt-newt"
  url "https://github.com/cwanda/incubator-mynewt-newt/tree/homebrew/archive/newt.tar.gz"
  sha256 "a96ccf3aff6afc0664c46bf368a3282243dad3dfbe619afcd2092b5fb60012d0"


  bottle: uneeded

  depends_on "go" => :build

  def install
    ENV["GOPATH"] = system "pwd"
    bin.install "newt"
  end

  end
end
