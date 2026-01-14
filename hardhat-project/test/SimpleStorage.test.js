const { expect } = require("chai");
const { ethers } = require("hardhat");

describe("SimpleStorage", function () {
  let simpleStorage;
  let owner;

  beforeEach(async function () {
    [owner] = await ethers.getSigners();

    const SimpleStorage = await ethers.getContractFactory("SimpleStorage");
    simpleStorage = await SimpleStorage.deploy();
    await simpleStorage.waitForDeployment();
  });

  it("Should store and retrieve a value", async function () {
    const valueToStore = 42;

    await simpleStorage.store(valueToStore);
    const storedValue = await simpleStorage.retrieve();

    expect(storedValue).to.equal(valueToStore);
  });

  it("Should emit ValueChanged event", async function () {
    const valueToStore = 123;

    await expect(simpleStorage.store(valueToStore))
      .to.emit(simpleStorage, "ValueChanged")
      .withArgs(valueToStore);
  });

  it("Should update value when stored multiple times", async function () {
    await simpleStorage.store(10);
    await simpleStorage.store(20);

    const storedValue = await simpleStorage.retrieve();
    expect(storedValue).to.equal(20);
  });
});
