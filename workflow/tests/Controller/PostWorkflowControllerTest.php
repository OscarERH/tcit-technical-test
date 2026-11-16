<?php

namespace App\Tests\Controller;

use PHPUnit\Framework\Attributes\DataProvider;
use Symfony\Bundle\FrameworkBundle\KernelBrowser;
use Symfony\Bundle\FrameworkBundle\Test\WebTestCase;

final class PostWorkflowControllerTest extends WebTestCase
{
    private KernelBrowser $client;

    protected function setUp(): void
    {
        $this->client = static::createClient();
    }

    public function testDefinitionDescribesTheLifecycle(): void
    {
        $this->client->request('GET', '/workflow/posts');

        self::assertResponseIsSuccessful();
        self::assertSame([
            'name' => 'post_publishing',
            'initialPlace' => 'draft',
            'places' => ['draft', 'published', 'archived'],
            'transitions' => [
                ['name' => 'publish', 'from' => ['draft'], 'to' => 'published'],
                ['name' => 'archive', 'from' => ['published'], 'to' => 'archived'],
                ['name' => 'restore', 'from' => ['archived'], 'to' => 'draft'],
            ],
        ], $this->json());
    }

    /** @param list<string> $enabledAfter */
    #[DataProvider('allowedTransitions')]
    public function testApplyMovesThePostToTheNextStatus(
        string $status,
        string $transition,
        string $expected,
        array $enabledAfter,
    ): void {
        $this->apply(['status' => $status, 'transition' => $transition]);

        self::assertResponseIsSuccessful();
        self::assertSame(['status' => $expected, 'enabledTransitions' => $enabledAfter], $this->json());
    }

    public static function allowedTransitions(): iterable
    {
        yield 'publish a draft' => ['draft', 'publish', 'published', ['archive']];
        yield 'archive a published post' => ['published', 'archive', 'archived', ['restore']];
        yield 'restore an archived post' => ['archived', 'restore', 'draft', ['publish']];
    }

    #[DataProvider('forbiddenTransitions')]
    public function testApplyRejectsTransitionsNotAllowedFromTheStatus(string $status, string $transition): void
    {
        $this->apply(['status' => $status, 'transition' => $transition]);

        self::assertResponseStatusCodeSame(409);
        self::assertSame(
            sprintf('Transition "%s" is not allowed from status "%s"', $transition, $status),
            $this->json()['error']['message'],
        );
    }

    public static function forbiddenTransitions(): iterable
    {
        yield 'archive a draft' => ['draft', 'archive'];
        yield 'publish twice' => ['published', 'publish'];
        yield 'publish an archived post' => ['archived', 'publish'];
    }

    public function testApplyRejectsUnknownStatus(): void
    {
        $this->apply(['status' => 'deleted', 'transition' => 'publish']);

        self::assertResponseStatusCodeSame(400);
        self::assertSame('Unknown status "deleted"', $this->json()['error']['message']);
    }

    public function testApplyRejectsUnknownTransition(): void
    {
        $this->apply(['status' => 'draft', 'transition' => 'explode']);

        self::assertResponseStatusCodeSame(400);
        self::assertSame('Unknown transition "explode"', $this->json()['error']['message']);
    }

    #[DataProvider('malformedBodies')]
    public function testApplyRejectsMalformedBodies(string $body): void
    {
        $this->client->request('POST', '/workflow/posts/apply', content: $body);

        self::assertResponseStatusCodeSame(400);
        self::assertArrayHasKey('message', $this->json()['error']);
    }

    public static function malformedBodies(): iterable
    {
        yield 'not JSON' => ['nope'];
        yield 'empty' => [''];
        yield 'missing transition' => ['{"status":"draft"}'];
        yield 'non-string values' => ['{"status":1,"transition":["publish"]}'];
        yield 'JSON scalar' => ['"draft"'];
    }

    public function testApplyOnlyAcceptsPost(): void
    {
        $this->client->request('GET', '/workflow/posts/apply');

        self::assertResponseStatusCodeSame(405);
    }

    /** @param array<string, mixed> $body */
    private function apply(array $body): void
    {
        $this->client->request('POST', '/workflow/posts/apply', content: json_encode($body, \JSON_THROW_ON_ERROR));
    }

    /** @return array<string, mixed> */
    private function json(): array
    {
        return json_decode($this->client->getResponse()->getContent(), true, flags: \JSON_THROW_ON_ERROR);
    }
}
