// Copyright (c) 2014 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tunny

//------------------------------------------------------------------------------

// workRequest is a struct containing context representing a workers intention
// to receive a work payload.
type request interface {
	getPayload() interface{}
}

type workRequest struct {
	payload interface{}

	// retChan is used to read the result from this worker.
	retChan chan<- interface{}

	// interruptFunc can be called to cancel a running job. When called it is no
	// longer necessary to read from retChan.
	interruptFunc func()
}

func (r *workRequest) getPayload() interface{} {
	return r.payload
}

type processRequest struct {
	workRequest

	// retChan is used to read the result from this worker.
	retChan chan<- interface{}
}

type runRequest struct {
	workRequest
}

//------------------------------------------------------------------------------

// workerWrapper takes a Worker implementation and wraps it within a goroutine
// and channel arrangement. The workerWrapper is responsible for managing the
// lifetime of both the Worker and the goroutine.
type workerWrapper struct {
	worker        Worker
	interruptChan chan struct{}

	// reqChan is NOT owned by this type, it is used to send requests for work.
	reqChan chan request

	workersChan chan chan request

	// closeChan can be closed in order to cleanly shutdown this worker.
	closeChan chan struct{}

	// closedChan is closed by the run() goroutine when it exits.
	closedChan chan struct{}
}

func newWorkerWrapper(
	workersChan chan chan request,
	worker Worker,
	i int,
) *workerWrapper {
	w := workerWrapper{
		worker:        worker,
		interruptChan: make(chan struct{}),
		workersChan:   workersChan,
		reqChan:       make(chan request),
		closeChan:     make(chan struct{}),
		closedChan:    make(chan struct{}),
	}

	go w.run(i)

	return &w
}

//------------------------------------------------------------------------------

func (w *workerWrapper) interrupt() {
	close(w.interruptChan)
	w.worker.Interrupt()
}

func (w *workerWrapper) run(i int) {
	defer func() {
		w.worker.Terminate()
		close(w.closedChan)
	}()

	for {
		// NOTE: Blocking here will prevent the worker from closing down.
		w.worker.BlockUntilReady()

		select {
		case w.workersChan <- w.reqChan:
			select {
			case req := <-w.reqChan:
				switch req.(type) {
				case *runRequest:
					w.worker.Run(req.getPayload())
				case *processRequest:
					processReq := req.(*processRequest)
					res := w.worker.Process(processReq.getPayload())
					select {
					case processReq.retChan <- res:
					case <-w.interruptChan:
						w.interruptChan = make(chan struct{})
					}
				}
			case <-w.interruptChan:
				w.interruptChan = make(chan struct{})
			}
		case <-w.closeChan:
			return
		}

	}
}

//------------------------------------------------------------------------------

func (w *workerWrapper) stop() {
	close(w.closeChan)
}

func (w *workerWrapper) join() {
	<-w.closedChan
}

//------------------------------------------------------------------------------
