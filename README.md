# MP4 Stream
## A Go-based utility for parsing MP4 files and outputting portions of the stream. It is intended to be used for pseudo-streaming.

This is my first attempt at a Go program. It is very rough and incomplete right now. At this point, it's only capable of parsing the boxes/atoms of an MP4 container. I will continue working on the functionality until it is capable of using this information to generate a new MP4 consisting of a specific time portion of the original MP4. Patches and suggestions welcome!

## Installing Go

See <http://golang.org/doc/install.html>.

## Installing MP4 Stream

    $ git clone git@github.com:bgentry/mp4_stream.git
    $ cd mp4_stream/src
    $ ./all.sh

## Try It Out

    $ mp4_stream -i ~/Movies/input_file.mp4

