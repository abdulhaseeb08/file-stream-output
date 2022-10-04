package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tinyzimmer/go-glib/glib"
	"github.com/tinyzimmer/go-gst/examples"
	"github.com/tinyzimmer/go-gst/gst"
	"github.com/tinyzimmer/go-gst/gst/app"
)

func buildAudioElements(pipeline *gst.Pipeline, stream bool) ([]*gst.Element, error) {

	if stream {
		elementsForAudio, err := gst.NewElementMany("openalsrc", "queue", "audioconvert", "audioresample", "audiorate", "capsfilter", "queue", "fdkaacenc", "queue", "tee")
		if err != nil {
			return nil, err
		}

		//Setting properties and caps
		if err := elementsForAudio[5].SetProperty("caps", gst.NewCapsFromString(
			"audio/x-raw, rate=48000, channels=2",
		)); err != nil {
			return nil, err
		}
		elementsForAudio[7].Set("bitrate", 128000)

		pipeline.AddMany(elementsForAudio...)
		//linking audio elements
		gst.ElementLinkMany(elementsForAudio...)

		return elementsForAudio, nil
	}

	elementsForAudio, err := gst.NewElementMany("openalsrc", "queue", "audioconvert", "audioresample", "audiorate", "capsfilter", "queue", "fdkaacenc", "queue")
	if err != nil {
		return nil, err
	}

	//Setting properties and caps
	if err := elementsForAudio[5].SetProperty("caps", gst.NewCapsFromString(
		"audio/x-raw, rate=48000, channels=2",
	)); err != nil {
		return nil, err
	}
	elementsForAudio[7].Set("bitrate", 128000)

	pipeline.AddMany(elementsForAudio...)
	//linking audio elements
	gst.ElementLinkMany(elementsForAudio...)

	return elementsForAudio, nil
}

func buildMux(pipeline *gst.Pipeline, name string) (*gst.Element, error) {
	if name == "mp4mux" {
		mux, err := gst.NewElement("mp4mux")
		if err != nil {
			return nil, err
		}
		pipeline.Add(mux)
		return mux, nil
	}

	mux, err := gst.NewElement("flvmux")
	if err != nil {
		return nil, err
	}
	pipeline.Add(mux)
	return mux, nil
}

func muxRequestPads(mux *gst.Element) (*gst.Pad, *gst.Pad) {
	audioPad := mux.GetRequestPad("audio_%u")
	if audioPad == nil {
		audioPad = mux.GetRequestPad("audio")
	}
	videoPad := mux.GetRequestPad("video_%u")
	if videoPad == nil {
		videoPad = mux.GetRequestPad("video")
	}

	return audioPad, videoPad

}

func buildVideoElements(pipeline *gst.Pipeline, stream bool) ([]*gst.Element, error) {

	if stream {
		elementsForVideo, err := gst.NewElementMany("v4l2src", "queue", "videoconvert", "videorate", "videoscale", "capsfilter", "queue", "x264enc", "h264parse", "capsfilter", "queue", "tee")
		if err != nil {
			return nil, err
		}

		//Setting properties and caps
		elementsForVideo[3].Set("silent", false)
		if err := elementsForVideo[5].SetProperty("caps", gst.NewCapsFromString(
			"video/x-raw, width=1280, height=720, framerate=30/1",
		)); err != nil {
			return nil, err
		}

		if err := elementsForVideo[9].SetProperty("caps", gst.NewCapsFromString(
			"video/x-h264, profile=high",
		)); err != nil {
			return nil, err
		}

		elementsForVideo[7].Set("speed-preset", 3)
		elementsForVideo[7].Set("tune", "zerolatency")
		elementsForVideo[7].Set("bitrate", 2500)
		elementsForVideo[7].Set("key-int-max", 100)

		pipeline.AddMany(elementsForVideo...)
		//linking video elements
		gst.ElementLinkMany(elementsForVideo...)

		return elementsForVideo, nil
	}

	elementsForVideo, err := gst.NewElementMany("v4l2src", "queue", "videoconvert", "videorate", "videoscale", "capsfilter", "queue", "x264enc", "queue")
	if err != nil {
		return nil, err
	}

	//Setting properties and caps
	elementsForVideo[3].Set("silent", false)
	if err := elementsForVideo[5].SetProperty("caps", gst.NewCapsFromString(
		"video/x-raw, width=1280, height=720, framerate=25/1",
	)); err != nil {
		return nil, err
	}

	elementsForVideo[7].Set("speed-preset", 3)
	elementsForVideo[7].Set("tune", "zerolatency")
	elementsForVideo[7].Set("bitrate", 3800)
	elementsForVideo[7].Set("key-int-max", 0)

	pipeline.AddMany(elementsForVideo...)
	//linking video elements
	gst.ElementLinkMany(elementsForVideo...)

	return elementsForVideo, nil

}

func buildPipeline() (*gst.Pipeline, error) {
	urls := os.Args[1:]
	argLen := len(urls)

	if argLen == 0 {
		stream := false
		//only build a pipeline that outputs a file

		//initialize gstreamer
		gst.Init(nil)

		//create a new pipeline
		pipeline, err := gst.NewPipeline("")
		if err != nil {
			return nil, err
		}

		//Build the video elements and add them to the pipeline and also link
		elementsForVideo, err := buildVideoElements(pipeline, stream)
		if err != nil {
			return nil, err
		}

		//video queue to link to mux
		videoQueue := elementsForVideo[len(elementsForVideo)-1]

		//Build the audio elements and add them to the pipeline and also link
		elementsForAudio, err := buildAudioElements(pipeline, stream)
		if err != nil {
			return nil, err
		}

		//audio queue to link to mux
		audioQueue := elementsForAudio[len(elementsForAudio)-1]

		muxFile, err := buildMux(pipeline, "mp4mux")
		if err != nil {
			return nil, err
		}

		//requesting mux sink pads
		muxFileAudioSink, muxFileVideoSink := muxRequestPads(muxFile)

		//Linking the mux sinks with the queues
		audioQueue.GetStaticPad("src").Link(muxFileAudioSink)
		videoQueue.GetStaticPad("src").Link(muxFileVideoSink)

		//create the file sink element
		filesink, err := gst.NewElement("filesink")
		if err != nil {
			return nil, err
		}
		filesink.Set("location", "file.mp4")
		pipeline.Add(filesink)

		//link the mux and the sink
		muxFile.Link(filesink)
		//Sending EOS event
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			for sig := range ch {
				switch sig {
				case syscall.SIGINT:
					fmt.Println("Sending EOS")
					pipeline.SendEvent(gst.NewEOSEvent())
					return
				}
			}
		}()

		return pipeline, nil

	}

	/************************************************************************/

	stream := true
	//initialize gstreamer
	gst.Init(nil)

	//create a new pipeline
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return nil, err
	}

	//Build the video elements and add them to the pipeline and also link
	elementsForVideo, err := buildVideoElements(pipeline, stream)
	if err != nil {
		return nil, err
	}
	videotee := elementsForVideo[len(elementsForVideo)-1]

	//Build the audio elements and add them to the pipeline and also link
	elementsForAudio, err := buildAudioElements(pipeline, stream)
	if err != nil {
		return nil, err
	}
	audiotee := elementsForAudio[len(elementsForAudio)-1]

	//Build both the muxes (one for file, one for streaming)
	muxFile, err := buildMux(pipeline, "mp4mux")
	if err != nil {
		return nil, err
	}

	muxStream, err := buildMux(pipeline, "flvmux")
	if err != nil {
		return nil, err
	}

	//requesting mux sink pads
	muxFileAudioSink, muxFileVideoSink := muxRequestPads(muxFile)
	muxStreamAudioSink, muxStreamVideoSink := muxRequestPads(muxStream)

	//creating queues for mux, we will link the sink pads of these queues with the audio and video tee elements
	muxQueues, err := gst.NewElementMany("queue", "queue", "queue", "queue")
	if err != nil {
		return nil, err
	}
	pipeline.AddMany(muxQueues...)
	muxFileQueueAudio := muxQueues[0]
	muxFileQueueVideo := muxQueues[1]
	muxStreamQueueAudio := muxQueues[2]
	muxStreamQueueVideo := muxQueues[3]

	//link the queues with the FileMux
	muxFileQueueAudio.GetStaticPad("src").Link(muxFileAudioSink)
	muxFileQueueVideo.GetStaticPad("src").Link(muxFileVideoSink)

	//Link the queues with the StreamMux
	muxStreamQueueAudio.GetStaticPad("src").Link(muxStreamAudioSink)
	muxStreamQueueVideo.GetStaticPad("src").Link(muxStreamVideoSink)

	//Requesting the source pads of tee
	teesrcFileAudio := audiotee.GetRequestPad("src_%u")
	teesrcFileVideo := videotee.GetRequestPad("src_%u")
	teesrcStreamAudio := audiotee.GetRequestPad("src_%u")
	teesrcStreamVideo := videotee.GetRequestPad("src_%u")

	//Link the queue sinks with the tee element (file)
	teesrcFileAudio.Link(muxFileQueueAudio.GetStaticPad("sink"))
	teesrcFileVideo.Link(muxFileQueueVideo.GetStaticPad("sink"))

	//Link the queue sinks with the tee element (stream)
	teesrcStreamAudio.Link(muxStreamQueueAudio.GetStaticPad("sink"))
	teesrcStreamVideo.Link(muxStreamQueueVideo.GetStaticPad("sink"))

	//Creating filesink, adding it to the pipline and linking to the mux
	filesink, err := gst.NewElement("filesink")
	if err != nil {
		return nil, err
	}
	filesink.Set("location", "file.mp4")
	pipeline.Add(filesink)
	muxFile.Link(filesink)

	if argLen == 1 {
		//Creating rtmp2sink, adding it to pipeline and linking to mux
		rtmpsink, err := gst.NewElement("rtmp2sink")
		if err != nil {
			return nil, err
		}
		rtmpsink.Set("location", urls[0])
		pipeline.Add(rtmpsink)
		muxStream.Link(rtmpsink)
	} else {
		flvtee, err := gst.NewElement("tee")
		if err != nil {
			return nil, err
		}
		pipeline.Add(flvtee)
		muxStream.Link(flvtee)

		for i, url := range urls {
			rtmpsink, err := gst.NewElementWithName("rtmp2sink", fmt.Sprintf("rtmpsink_%d", i))
			if err != nil {
				return nil, err
			}
			rtmpsink.Set("location", url)
			queue, err := gst.NewElementWithName("queue", fmt.Sprintf("queue_%d", i))
			if err != nil {
				return nil, err
			}
			pipeline.AddMany(rtmpsink, queue)
			queue.Link(rtmpsink)

			//connect tee pad with queue
			flvTeePad := flvtee.GetRequestPad("src_%u")
			flvTeePad.Link(queue.GetStaticPad("sink"))
		}

	}

	//Sending EOS event
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		for sig := range ch {
			switch sig {
			case syscall.SIGINT:
				fmt.Println("Sending EOS")
				pipeline.SendEvent(gst.NewEOSEvent())
				return
			}
		}
	}()

	return pipeline, nil

}

func handleMessage(msg *gst.Message) error {
	switch msg.Type() {
	case gst.MessageEOS:
		return app.ErrEOS
	case gst.MessageError:
		gerr := msg.ParseError()
		if debug := gerr.DebugString(); debug != "" {
			fmt.Println(debug)
		}
		return gerr
	}
	return nil
}

func mainLoop(loop *glib.MainLoop, pipeline *gst.Pipeline) error {
	// Start the pipeline

	pipeline.Ref()
	defer pipeline.Unref()

	pipeline.SetState(gst.StatePlaying)

	// Retrieve the bus from the pipeline and add a watch function
	pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		if err := handleMessage(msg); err != nil {
			fmt.Println(err)
			loop.Quit()
			return false
		}
		return true
	})

	loop.Run()

	return nil
}

func main() {
	examples.RunLoop(func(loop *glib.MainLoop) error {
		var pipeline *gst.Pipeline
		var err error
		if pipeline, err = buildPipeline(); err != nil {
			return err
		}
		return mainLoop(loop, pipeline)
	})
}
