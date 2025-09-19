Common principles:

* Ask for input for critical decissions, do not make assumptions if they are not backed up by existing code or defined as requirements. Provide preference with justification.
* ALWAYS ensure changes to the code will not bring any compilation or lint errors (using golangci-lint), including development and testing code.
* Whenever it applies, FIRST start with unit tests (TDD) and also defining the business contract in the business code to enable the tests to compile. DO NOT use Skip() to avoid the test failure.  Once ALL tests are completely implemented and compilation succeeds but tests fail (as expected), THEN proceed with the business logic code and then run the existing tests.
* All code must be backed up by at least ONE business requirement, whereas tests or implementation code.
* ALWAYS attempt to use structured field values and AVOID using any or interface{} instead unless it collides with any of the other principles.
* Be clear, realistic and avoid superlatives in your interactions.

Development principles:
* AVOID duplication and REUSE existin code. REFACTOR code if necessary to ensure maximum reusability unless it creates a complex and difficult to maintain structure. ALWAYS ask for input for critical decissions in that scenario
* Ensure functionality aligns with business requirements. DO NOT implement code that is not supported or backed up by a requirement
* Integrate all new code with existing code: Any new code that is not integrated with the main code and is not backed up by a requirement should be removed before completing this task.
* ALWAYS log errors, never ignore them.
* Ensure NO errors are ignored.
* Avoid duplicating structure names. ALWAYS use unique names that align best with the business purpose and can't lead to confusion.
* DO NOT use local type definitions to resolve import cycles. Instead, leverage on shared types or move existing local types to the shared types package and updated references across the code to use them.
* AVOID implementing backwards compatibility support. We have not yet released and there is no need to support legacy code.


Testing principles:
* Reuse test framework code unless not available, in which case ask for guidance and wait for response. Propose suggestions.
* ALWAYS avoid the null-testing anti-pattern
* Use ginkgo/gomega BDD test framework.
* Use existing mocks and fake client for K8s interaction.
* Ensure all business requirements have at least a unit test backing them up.
* DO NOT test the implementation of the business requirement, test the actual business requirement expectations.
* Ensure test assertions are aligned with business requirements.
* Assertions MUST be backed on business outcomes. AVOID weak business validations (not nil, > 0, < 0, not empty, etc...). Whenever string is randomly generated, use substring validation (contains) or numbers (range). Weak assertions don't provide a strong business requirements confidence.
* Prefer to reuse existing mocks and extend them than create local ones. When no mocks exist, create one in the shared directory where all the other mocks reside.
* Ensure all errors are captured and validated correctly.
* When triaging test run failures START by investigating the root cause and providing a confidence assessment on the potential fixes, including the most likely recommendation. DO NOT apply fixes until the solutions are reviewed.
* Only use mocks for logic validation. Integration tests should focus on avoiding mocks as much as possible unless the test scenario becomes too complex, in which case it's best handled in an e2e setup

After completing task:
* Whenever applies, ensure code builds without errors related to changes made during this session. Address any error found based on the previous principles.
* Propose enhancements with a confidence level >=60% based on the current milestone scope. DO NOT implement anything unless otherwise stated.
* Ensure no new lint errors are found due to the changes introduced by this action, such as unusedparam or unusedfunc types.
* Provide a summary of the coverage of Business Requirements tested based on the scope of testing (unit, integration or end to end).
