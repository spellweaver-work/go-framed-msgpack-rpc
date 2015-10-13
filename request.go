package rpc

type request interface {
	Message() *message
	Reply(encoder, LogInterface) error
	Serve(byteReadingDecoder, encoder, *ServeHandlerDescription, WrapErrorFunc, LogInterface)
	LogInvocation(log LogInterface, err error, arg interface{})
	LogCompletion(log LogInterface, err error)
}

type requestImpl struct {
	message
}

func (req *requestImpl) Message() *message {
	return &req.message
}

func (req *requestImpl) getArg(receiver decoder, handler *ServeHandlerDescription) (interface{}, error) {
	arg := handler.MakeArg()
	err := decodeMessage(receiver, req.Message(), arg)
	return arg, err
}

type callRequest struct {
	requestImpl
}

func newCallRequest() *callRequest {
	r := &callRequest{
		requestImpl: requestImpl{
			message: message{
				remainingFields: 3,
			},
		},
	}
	r.decodeSlots = []interface{}{
		&r.seqno,
		&r.method,
	}
	return r
}

func (r *callRequest) LogInvocation(log LogInterface, err error, arg interface{}) {
	log.ServerCall(r.seqno, r.method, err, arg)
}

func (r *callRequest) LogCompletion(log LogInterface, err error) {
	log.ServerReply(r.seqno, r.method, err, r.res)
}

func (r *callRequest) Reply(enc encoder, log LogInterface) error {
	v := []interface{}{
		MethodResponse,
		r.seqno,
		r.err,
		r.res,
	}
	err := enc.Encode(v)
	if err != nil {
		log.Warning("Reply error for %d: %s", r.seqno, err.Error())
	}
	return err
}

func (r *callRequest) Serve(receiver byteReadingDecoder, transmitter encoder, handler *ServeHandlerDescription, wrapErrorFunc WrapErrorFunc, log LogInterface) {

	prof := log.StartProfiler("serve %s", r.method)
	arg, err := r.getArg(receiver, handler)

	go func() {
		r.LogInvocation(log, err, arg)
		if err != nil {
			r.err = wrapError(wrapErrorFunc, err)
		} else {
			res, err := handler.Handler(arg)
			r.err = wrapError(wrapErrorFunc, err)
			r.res = res
		}
		prof.Stop()
		r.LogCompletion(log, err)
		r.Reply(transmitter, log)
	}()
}

type notifyRequest struct {
	requestImpl
}

func newNotifyRequest() *notifyRequest {
	r := &notifyRequest{
		requestImpl: requestImpl{
			message: message{
				remainingFields: 2,
			},
		},
	}
	r.decodeSlots = []interface{}{
		&r.method,
	}
	return r
}

func (r *notifyRequest) LogInvocation(log LogInterface, err error, arg interface{}) {
	log.ServerNotifyCall(r.method, err, arg)
}

func (r *notifyRequest) LogCompletion(log LogInterface, err error) {
	log.ServerNotifyComplete(r.method, err)
}

func (r *notifyRequest) Reply(enc encoder, log LogInterface) error {
	return nil
}

func (r *notifyRequest) Serve(receiver byteReadingDecoder, transmitter encoder, handler *ServeHandlerDescription, wrapErrorFunc WrapErrorFunc, log LogInterface) {

	prof := log.StartProfiler("serve-notify %s", r.method)
	arg, err := r.getArg(receiver, handler)

	go func() {
		r.LogInvocation(log, err, arg)
		if err == nil {
			_, err = handler.Handler(arg)
		}
		prof.Stop()
		r.LogCompletion(log, err)
	}()
}

type cancelRequest struct {
	requestImpl
}

func newCancelRequest() *cancelRequest {
	r := &cancelRequest{
		requestImpl: requestImpl{
			message: message{
				remainingFields: 2,
			},
		},
	}
	r.decodeSlots = []interface{}{
		&r.seqno,
		&r.method,
	}
	return r
}

func (r *cancelRequest) LogInvocation(log LogInterface, err error, arg interface{}) {
	log.ServerCancelCall(r.seqno, r.method)
}

func (r *cancelRequest) LogCompletion(log LogInterface, err error) {
}

func (r *cancelRequest) Reply(enc encoder, log LogInterface) error {
	return nil
}

func (r *cancelRequest) Serve(receiver byteReadingDecoder, transmitter encoder, handler *ServeHandlerDescription, wrapErrorFunc WrapErrorFunc, log LogInterface) {
	r.LogInvocation(log, nil, nil)
}

func newRequest(methodType MethodType) request {
	switch methodType {
	case MethodCall:
		return newCallRequest()
	case MethodNotify:
		return newNotifyRequest()
	case MethodCancel:
		return newCancelRequest()
	}
	return nil
}

func decodeIntoRequest(dec decoder, r request) error {
	m := r.Message()
	for _, s := range m.decodeSlots {
		if err := decodeMessage(dec, m, s); err != nil {
			return err
		}
	}
	return nil
}
