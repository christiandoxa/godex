package deepseek

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	compactgateway "github.com/christiandoxa/godex/internal/gateway/compact"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	contentTypeHeader              = "Content-Type"
	defaultAPIURL                  = "https://api.deepseek.com"
	defaultBetaBaseURL             = "https://api.deepseek.com/beta"
	anthropicVersion               = "2023-06-01"
	bodyMaxBytes                   = 8 << 20
	translatedResponseBodyMaxBytes = deepSeekResponseArgumentsMaxBytes + (4 << 20)
	streamEventMaxBytes            = 1 << 20
	nativeMessagesMaxBytes         = 4 << 20
)

type RuntimeTransport struct {
	client        *http.Client
	upstream      *url.URL
	betaUpstream  *url.URL
	apiKey        string
	options       RequestOptions
	conversations deepSeekConversationStore
}

func NewRuntimeTransport(apiURL, apiKey string, client *http.Client) (*RuntimeTransport, error) {
	return NewRuntimeTransportWithOptions(apiURL, apiKey, RequestOptions{}, client)
}

func NewRuntimeTransportWithOptions(apiURL, apiKey string, options RequestOptions, client *http.Client) (*RuntimeTransport, error) {
	if strings.TrimSpace(apiURL) == "" {
		apiURL = defaultAPIURL
	}
	parsed, err := validateRuntimeURL(apiURL)
	if err != nil {
		return nil, err
	}
	if options.BetaBaseURL == "" {
		options.BetaBaseURL = defaultBetaBaseURL
	}
	if options.SSELookaheadTimeout <= 0 {
		options.SSELookaheadTimeout = defaultSSELookaheadTimeout
	}
	if options.StreamIdleTimeout <= 0 {
		options.StreamIdleTimeout = defaultSSEStreamIdleTimeout
	}
	betaUpstream, err := validateRuntimeURL(options.BetaBaseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("DeepSeek API credential is unavailable")
	}
	return &RuntimeTransport{
		client: cloneClient(client), upstream: parsed, betaUpstream: betaUpstream, apiKey: apiKey, options: options,
		conversations: newDeepSeekConversationStore(),
	}, nil
}

func (transport *RuntimeTransport) Execute(ctx context.Context, input proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	current, err := runtimeRoute(input.Path)
	if err != nil {
		return nil, err
	}
	switch current.kind {
	case routeModelsList, routeModelsSingle:
		return modelsResponse(input.Method, current)
	case routeCompact:
		return compactgateway.LocalFallback(input.Body, deepSeekProviderKey, "local-policy")
	case routeResponses:
		return transport.executeResponses(ctx, input, current)
	default:
		return transport.executePassthrough(ctx, input, current)
	}
}

type deepSeekResponseAttempt struct {
	body                 []byte
	route                route
	nativeMessages       bool
	metadata             map[string]any
	conversationMessages []any
	conversations        deepSeekConversationStore
}

type deepSeekPrecommitState struct {
	failure   *proxymodel.PrecommitFailure
	retry     bool
	retryUsed bool
	committed bool
}

func (transport *RuntimeTransport) executeResponses(ctx context.Context, input proxymodel.Request, current route) (*proxymodel.Response, error) {
	models := providerentity.ModelFallbackChain(deepSeekProviderKey, requestModel(input.Body))
	if len(models) == 0 {
		models = []string{"deepseek-v4-pro", "deepseek-v4-flash"}
	}
	firstEventRetryUsed := input.FirstEventRetryUsed
	conversations := transport.conversationsForRequest(input)
	for index, candidate := range models {
		attempt, err := transport.prepareResponseAttempt(input, current, candidate, conversations)
		if err != nil {
			return nil, err
		}
		response, err := transport.send(ctx, input, attempt.route, attempt.body)
		if err != nil {
			return nil, err
		}
		final, retry, retryUsed, err := transport.finishResponseAttempt(
			ctx, input, response, attempt, firstEventRetryUsed, index+1 < len(models),
		)
		firstEventRetryUsed = retryUsed
		if err != nil {
			return nil, err
		}
		if retry {
			continue
		}
		return final, nil
	}
	return nil, errors.New("DeepSeek runtime model fallback produced no attempts")
}

func (transport *RuntimeTransport) prepareResponseAttempt(input proxymodel.Request, current route, candidate string, conversations deepSeekConversationStore) (deepSeekResponseAttempt, error) {
	history := deepSeekConversationHistoryForRequest(input.Body, conversations)
	translated, err := translateResponsesRequestWithHistory(input.Body, RequestOptions{
		Model: candidate, StrictTools: transport.options.StrictTools, WebSearchMode: transport.options.WebSearchMode,
	}, history)
	if err != nil {
		return deepSeekResponseAttempt{}, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	attempt := deepSeekResponseAttempt{
		body: translated.Body, route: current, metadata: translated.ResponseMetadata, conversations: conversations,
		nativeMessages: nativeMessagesMode(transport.options.WebSearchMode) && nativeMessagesContext(translated.Body),
	}
	attempt.conversationMessages = deepSeekTranslatedConversationMessages(translated.Body)
	if !attempt.nativeMessages {
		return attempt, nil
	}
	attempt.body, err = deepSeekAnthropicRequest(translated.Body)
	if err == nil {
		attempt.route = route{kind: routeMessages}
		return attempt, nil
	}
	if !nativeMessagesFallbackAllowed(transport.options.WebSearchMode) || !deepSeekNativeFallbackSafe(err) {
		return deepSeekResponseAttempt{}, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	attempt.body, err = deepSeekChatFallbackBody(translated.Body)
	if err != nil {
		return deepSeekResponseAttempt{}, err
	}
	attempt.nativeMessages = false
	return attempt, nil
}

func (transport *RuntimeTransport) finishResponseAttempt(
	ctx context.Context,
	input proxymodel.Request,
	response *http.Response,
	attempt deepSeekResponseAttempt,
	firstEventRetryUsed bool,
	hasNextModel bool,
) (*proxymodel.Response, bool, bool, error) {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if attempt.nativeMessages {
			return transport.finishNativeResponse(ctx, input, response, attempt, firstEventRetryUsed, hasNextModel)
		}
		translated, err := translateResponseWithConversation(
			response, attempt.metadata, attempt.conversations, attempt.conversationMessages, input.RequestID,
		)
		return translated, false, firstEventRetryUsed, err
	}
	buffered, err := bufferError(response)
	if err != nil {
		return nil, false, firstEventRetryUsed, err
	}
	classification := providerentity.ClassifyError(buffered.StatusCode, buffered.body)
	if hasNextModel && providerentity.RetryableAcrossModels(classification.Class) {
		return nil, true, firstEventRetryUsed, nil
	}
	return buffered.proxyResponse(), false, firstEventRetryUsed, nil
}

func (transport *RuntimeTransport) finishNativeResponse(
	ctx context.Context,
	input proxymodel.Request,
	response *http.Response,
	attempt deepSeekResponseAttempt,
	firstEventRetryUsed bool,
	hasNextModel bool,
) (*proxymodel.Response, bool, bool, error) {
	precommit, err := transport.inspectNativePrecommit(ctx, response, firstEventRetryUsed, hasNextModel)
	if err != nil {
		return nil, false, precommit.retryUsed, err
	}
	if precommit.retry {
		return nil, true, precommit.retryUsed, nil
	}
	translated, err := translateAnthropicResponseWithConversation(
		response, attempt.metadata, input.RequestID, attempt.conversations, attempt.conversationMessages,
	)
	if err != nil {
		return nil, false, precommit.retryUsed, err
	}
	translated.FirstEventRetryUsed = precommit.retryUsed
	translated.FirstEventCommitted = precommit.committed
	translated.PrecommitFailure = precommit.failure
	return translated, false, precommit.retryUsed, nil
}

func (transport *RuntimeTransport) inspectNativePrecommit(
	ctx context.Context,
	response *http.Response,
	firstEventRetryUsed bool,
	hasNextModel bool,
) (deepSeekPrecommitState, error) {
	state := deepSeekPrecommitState{retryUsed: firstEventRetryUsed}
	if !strings.Contains(strings.ToLower(response.Header.Get(contentTypeHeader)), "text/event-stream") {
		return state, nil
	}
	state.committed = true
	upstreamBody := response.Body
	replayed, firstEvent, err := peekAnthropicFirstEvent(ctx, upstreamBody, transport.options.SSELookaheadTimeout, transport.options.StreamIdleTimeout)
	response.Body = replayed
	if ctx.Err() != nil {
		_ = upstreamBody.Close()
		return state, ctx.Err()
	}
	if err != nil && !errors.Is(err, io.EOF) {
		if !state.retryUsed && hasNextModel {
			_ = response.Body.Close()
			state.retry = true
			state.retryUsed = true
			return state, nil
		}
		state.failure = &proxymodel.PrecommitFailure{Transport: true}
	} else {
		state = classifyNativeFirstEvent(state, firstEvent, hasNextModel, response)
		if state.retry {
			return state, nil
		}
	}
	if state.failure != nil {
		classification := providerentity.ClassifyProviderCode(state.failure.Code)
		if state.failure.Transport {
			classification = providerentity.ClassifyError(http.StatusBadGateway, nil)
		}
		state.committed = state.retryUsed || !providerentity.RetryableAcrossCredentials(classification.Class)
	}
	return state, nil
}

func classifyNativeFirstEvent(
	state deepSeekPrecommitState,
	firstEvent []byte,
	hasNextModel bool,
	response *http.Response,
) deepSeekPrecommitState {
	code, classification, isError := providerentity.ClassifyFirstEventError(firstEvent)
	if !isError {
		return state
	}
	if !state.retryUsed && hasNextModel && providerentity.RetryableAcrossModels(classification.Class) {
		_ = response.Body.Close()
		state.retry = true
		state.retryUsed = true
		return state
	}
	state.failure = &proxymodel.PrecommitFailure{Code: code}
	return state
}

func (transport *RuntimeTransport) executePassthrough(ctx context.Context, input proxymodel.Request, current route) (*proxymodel.Response, error) {
	response, err := transport.send(ctx, input, current, input.Body)
	if err != nil {
		return nil, err
	}
	return proxyResponse(response), nil
}

func (transport *RuntimeTransport) send(ctx context.Context, input proxymodel.Request, current route, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, input.Method, transport.target(current, input.RawQuery), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("create DeepSeek upstream request")
	}
	applyHeaders(request.Header, input.Header, transport.apiKey, current.kind == routeMessages)
	return transport.client.Do(request)
}

func (transport *RuntimeTransport) target(current route, rawQuery string) string {
	target := *transport.upstream
	if current.kind == routeResponses && transport.options.StrictTools {
		target = *transport.betaUpstream
	}
	base := strings.TrimRight(target.Path, "/")
	switch current.kind {
	case routeResponses, routeChat:
		target.Path = base + "/chat/completions"
	case routeMessages:
		target.Path = deepSeekMessagesPath(base)
	}
	target.RawPath = ""
	target.RawQuery = rawQuery
	return target.String()
}

func deepSeekMessagesPath(basePath string) string {
	basePath = strings.TrimRight(basePath, "/")
	if strings.HasSuffix(basePath, "/anthropic/v1") {
		return basePath + "/messages"
	}
	if strings.HasSuffix(basePath, "/anthropic") {
		return basePath + "/v1/messages"
	}
	for _, suffix := range []string{"/v1", "/beta"} {
		if strings.HasSuffix(basePath, suffix) {
			basePath = strings.TrimSuffix(basePath, suffix)
			break
		}
	}
	return basePath + "/anthropic/v1/messages"
}

func applyHeaders(destination, source http.Header, apiKey string, nativeMessages bool) {
	destination.Set(contentTypeHeader, "application/json")
	destination.Set("Accept-Encoding", "identity")
	destination.Set("Accept", "text/event-stream, application/json")
	if nativeMessages {
		destination.Set("x-api-key", apiKey)
		destination.Set("anthropic-version", anthropicVersion)
	} else {
		destination.Set("Authorization", "Bearer "+apiKey)
	}
	if userAgent := source.Get("User-Agent"); userAgent != "" {
		destination.Set("User-Agent", userAgent)
	}
	for _, name := range []string{"traceparent", "tracestate", "baggage"} {
		if value := source.Get(name); value != "" {
			destination.Set(name, value)
		}
	}
}
