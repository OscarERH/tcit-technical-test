<?php

namespace App\Model;

/**
 * Subject of the post_publishing workflow.
 *
 * Posts live in the Go API (Cloud Spanner); this service is stateless and only
 * receives the current status of a post to decide where it can go next.
 */
final class PostState
{
    public function __construct(private string $status)
    {
    }

    public function getStatus(): string
    {
        return $this->status;
    }

    public function setStatus(string $status): void
    {
        $this->status = $status;
    }
}
