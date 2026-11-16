<?php

namespace App\Controller;

use App\Model\PostState;
use Symfony\Component\DependencyInjection\Attribute\Target;
use Symfony\Component\HttpFoundation\JsonResponse;
use Symfony\Component\HttpFoundation\Request;
use Symfony\Component\HttpFoundation\Response;
use Symfony\Component\Routing\Attribute\Route;
use Symfony\Component\Workflow\WorkflowInterface;

#[Route('/workflow/posts')]
final class PostWorkflowController
{
    public function __construct(
        #[Target('post_publishing')]
        private readonly WorkflowInterface $workflow,
    ) {
    }

    /** Describes the workflow: places, initial place and transitions. */
    #[Route('', methods: ['GET'])]
    public function definition(): JsonResponse
    {
        $definition = $this->workflow->getDefinition();

        $transitions = [];
        foreach ($definition->getTransitions() as $transition) {
            $name = $transition->getName();
            $transitions[$name] ??= ['name' => $name, 'from' => [], 'to' => $transition->getTos()[0]];
            array_push($transitions[$name]['from'], ...$transition->getFroms());
        }

        return new JsonResponse([
            'name' => $this->workflow->getName(),
            'initialPlace' => $definition->getInitialPlaces()[0],
            'places' => array_values($definition->getPlaces()),
            'transitions' => array_values($transitions),
        ]);
    }

    /** Applies a transition to a status and returns the resulting status. */
    #[Route('/apply', methods: ['POST'])]
    public function apply(Request $request): JsonResponse
    {
        $body = json_decode($request->getContent(), true);
        $status = is_array($body) ? ($body['status'] ?? null) : null;
        $transition = is_array($body) ? ($body['transition'] ?? null) : null;

        if (!is_string($status) || !is_string($transition)) {
            return $this->error(Response::HTTP_BAD_REQUEST, '"status" and "transition" are required strings');
        }

        $definition = $this->workflow->getDefinition();
        if (!in_array($status, $definition->getPlaces(), true)) {
            return $this->error(Response::HTTP_BAD_REQUEST, sprintf('Unknown status "%s"', $status));
        }

        $known = array_map(static fn ($t) => $t->getName(), $definition->getTransitions());
        if (!in_array($transition, $known, true)) {
            return $this->error(Response::HTTP_BAD_REQUEST, sprintf('Unknown transition "%s"', $transition));
        }

        $post = new PostState($status);
        if (!$this->workflow->can($post, $transition)) {
            return $this->error(
                Response::HTTP_CONFLICT,
                sprintf('Transition "%s" is not allowed from status "%s"', $transition, $status),
            );
        }

        $this->workflow->apply($post, $transition);

        return new JsonResponse([
            'status' => $post->getStatus(),
            'enabledTransitions' => array_map(
                static fn ($t) => $t->getName(),
                $this->workflow->getEnabledTransitions($post),
            ),
        ]);
    }

    private function error(int $status, string $message): JsonResponse
    {
        return new JsonResponse(['error' => ['message' => $message]], $status);
    }
}
